package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"secrio/internal/domain"
)

// Secret records: ID=secret id, ParentID=env id, sealed under the ENV key.
// Lookup by name decrypts every secret of the environment (scale target:
// hundreds per environment).

var knownSecretTypes = map[domain.SecretType]bool{
	domain.TypeText: true, domain.TypePassword: true, domain.TypeAPIKey: true, domain.TypeToken: true,
	domain.TypeURL: true, domain.TypePrivateKey: true, domain.TypeCertificate: true, domain.TypeJSON: true,
}

// SetSecret creates or replaces (by exact, case-sensitive name) a secret.
// An empty typ means domain.TypeText.
func (u *Unlocked) SetSecret(ctx context.Context, envID domain.ID[domain.Environment], name domain.SecretName,
	value []byte, typ domain.SecretType, desc string) (domain.Secret, error) {
	if _, err := domain.ParseSecretName(string(name)); err != nil {
		return domain.Secret{}, err
	}
	if typ == "" {
		typ = domain.TypeText
	}
	if !knownSecretTypes[typ] {
		return domain.Secret{}, fmt.Errorf("%w: unknown secret type", domain.ErrInvalidName)
	}
	now := u.clock.Now().UTC()
	s := domain.Secret{ID: domain.NewID[domain.Secret](), EnvID: envID, Name: name, Value: bytes.Clone(value),
		Description: desc, Type: typ, CreatedAt: now, UpdatedAt: now}
	err := u.store.Tx(ctx, func(tx RecordStore) error {
		key, err := u.envKey(ctx, tx, envID)
		if err != nil {
			return err
		}
		defer zeroKey(&key)
		existing, err := u.loadSecrets(ctx, tx, key, envID)
		if err != nil {
			return err
		}
		if old, ok := findSecret(existing, name); ok {
			s.ID, s.CreatedAt = old.ID, old.CreatedAt
		}
		return u.putSecretTx(ctx, tx, key, s)
	})
	if err != nil {
		return domain.Secret{}, err
	}
	return s, nil
}

func (u *Unlocked) GetSecret(ctx context.Context, envID domain.ID[domain.Environment], name domain.SecretName) (domain.Secret, error) {
	secrets, err := u.ListSecrets(ctx, envID)
	if err != nil {
		return domain.Secret{}, err
	}
	s, ok := findSecret(secrets, name)
	if !ok {
		return domain.Secret{}, fmt.Errorf("secret: %w", domain.ErrNotFound)
	}
	return s, nil
}

// ListSecrets returns the environment's secrets sorted by name.
func (u *Unlocked) ListSecrets(ctx context.Context, envID domain.ID[domain.Environment]) ([]domain.Secret, error) {
	key, err := u.envKey(ctx, u.store, envID)
	if err != nil {
		return nil, err
	}
	defer zeroKey(&key)
	secrets, err := u.loadSecrets(ctx, u.store, key, envID)
	if err != nil {
		return nil, err
	}
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	return secrets, nil
}

func (u *Unlocked) DeleteSecret(ctx context.Context, envID domain.ID[domain.Environment], name domain.SecretName) error {
	return u.store.Tx(ctx, func(tx RecordStore) error {
		key, err := u.envKey(ctx, tx, envID)
		if err != nil {
			return err
		}
		defer zeroKey(&key)
		secrets, err := u.loadSecrets(ctx, tx, key, envID)
		if err != nil {
			return err
		}
		s, ok := findSecret(secrets, name)
		if !ok {
			return fmt.Errorf("secret: %w", domain.ErrNotFound)
		}
		return tx.Delete(ctx, KindSecret, string(s.ID))
	})
}

func (u *Unlocked) putSecretTx(ctx context.Context, tx RecordStore, envKey domain.Key, s domain.Secret) error {
	return putSealed(ctx, tx, u, envKey, KindSecret, string(s.ID), string(s.EnvID), s)
}

// envKey unwraps the key of an existing environment; an unknown environment
// is domain.ErrNotFound. The caller zeroes the key.
func (u *Unlocked) envKey(ctx context.Context, rs RecordStore, envID domain.ID[domain.Environment]) (domain.Key, error) {
	if _, err := rs.Get(ctx, KindEnvironment, string(envID)); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Key{}, fmt.Errorf("environment: %w", domain.ErrNotFound)
		}
		return domain.Key{}, err
	}
	r, err := rs.Get(ctx, KindEnvKey, string(envID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) { // env exists without its key
			return domain.Key{}, fmt.Errorf("%w: environment key missing", domain.ErrCorrupt)
		}
		return domain.Key{}, err
	}
	return unwrapEnvKey(u.c, u.key, string(envID), r.Blob)
}

// loadSecrets decrypts every secret of envID under its env key.
func (u *Unlocked) loadSecrets(ctx context.Context, rs RecordStore, envKey domain.Key, envID domain.ID[domain.Environment]) ([]domain.Secret, error) {
	secrets, err := listRecords[domain.Secret](ctx, rs, u.c, envKey, KindSecret, string(envID))
	if err != nil {
		return nil, err
	}
	for _, s := range secrets {
		if s.EnvID != envID {
			return nil, fmt.Errorf("%w: secret environment mismatch", domain.ErrCorrupt)
		}
	}
	return secrets, nil
}

func findSecret(secrets []domain.Secret, name domain.SecretName) (domain.Secret, bool) {
	for _, s := range secrets {
		if s.Name == name {
			return s, true
		}
	}
	return domain.Secret{}, false
}
