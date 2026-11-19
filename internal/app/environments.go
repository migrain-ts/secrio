package app

import (
	"context"
	"fmt"
	"sort"

	"secrio/internal/domain"
)

// Environments: see the record layout in structure.go.

// CreateEnvironment creates an environment with its own new env key.
func (u *Unlocked) CreateEnvironment(ctx context.Context, projectID domain.ID[domain.Project], name string) (domain.Environment, error) {
	if err := validName(name); err != nil {
		return domain.Environment{}, err
	}
	env := domain.Environment{ID: domain.NewID[domain.Environment](), ProjectID: projectID, Name: name}
	err := u.store.Tx(ctx, func(tx RecordStore) error {
		envs, err := u.listEnvsTx(ctx, tx, projectID)
		if err != nil {
			return err
		}
		for _, e := range envs {
			if e.Name == name {
				return fmt.Errorf("environment name: %w", domain.ErrDuplicate)
			}
		}
		key, err := u.putEnvTx(ctx, tx, env)
		if err != nil {
			return err
		}
		defer zeroKey(&key)
		return u.recordHistoryTx(ctx, tx, key, historyRec{HistoryEntry: HistoryEntry{EnvID: env.ID, Op: OpEnvCreate}})
	})
	if err != nil {
		return domain.Environment{}, err
	}
	return env, nil
}

// ListEnvironments returns the project's environments sorted by name.
func (u *Unlocked) ListEnvironments(ctx context.Context, projectID domain.ID[domain.Project]) ([]domain.Environment, error) {
	envs, err := u.listEnvsTx(ctx, u.store, projectID)
	if err != nil {
		return nil, err
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}

// DeleteEnvironment deletes the environment, its env key and its secrets.
func (u *Unlocked) DeleteEnvironment(ctx context.Context, envID domain.ID[domain.Environment]) error {
	return u.store.Tx(ctx, func(tx RecordStore) error {
		if _, err := getRecord[domain.Environment](ctx, tx, u.c, u.key, KindEnvironment, string(envID)); err != nil {
			return err
		}
		return u.deleteEnvTx(ctx, tx, envID)
	})
}

// listEnvsTx lists a project's environments; an unknown project is
// domain.ErrNotFound.
func (u *Unlocked) listEnvsTx(ctx context.Context, rs RecordStore, projectID domain.ID[domain.Project]) ([]domain.Environment, error) {
	if _, err := getRecord[domain.Project](ctx, rs, u.c, u.key, KindProject, string(projectID)); err != nil {
		return nil, err
	}
	return listRecords[domain.Environment](ctx, rs, u.c, u.key, KindEnvironment, string(projectID))
}

// putEnvTx stores a new environment and a freshly generated env key, which
// it returns (the caller zeroes it).
func (u *Unlocked) putEnvTx(ctx context.Context, tx RecordStore, env domain.Environment) (domain.Key, error) {
	key, wrapped, err := newEnvKey(u.c, u.key, string(env.ID))
	if err != nil {
		return domain.Key{}, err
	}
	err = tx.Put(ctx, Record{Kind: KindEnvKey, ID: string(env.ID), ParentID: string(env.ProjectID),
		Blob: wrapped, UpdatedAt: u.clock.Now().UTC()})
	if err == nil {
		err = putSealed(ctx, tx, u, u.key, KindEnvironment, string(env.ID), string(env.ProjectID), env)
	}
	if err != nil {
		zeroKey(&key)
		return domain.Key{}, err
	}
	return key, nil
}

// deleteEnvTx deletes an environment's secrets, history, env key and record.
func (u *Unlocked) deleteEnvTx(ctx context.Context, tx RecordStore, envID domain.ID[domain.Environment]) error {
	secrets, err := tx.List(ctx, KindSecret, string(envID))
	if err != nil {
		return err
	}
	for _, s := range secrets {
		if err := tx.Delete(ctx, KindSecret, s.ID); err != nil {
			return err
		}
	}
	hist, err := tx.List(ctx, KindHistory, string(envID))
	if err != nil {
		return err
	}
	for _, h := range hist {
		if err := tx.Delete(ctx, KindHistory, h.ID); err != nil {
			return err
		}
	}
	if err := tx.Delete(ctx, KindEnvKey, string(envID)); err != nil {
		return err
	}
	return tx.Delete(ctx, KindEnvironment, string(envID))
}

// copyEnvTx creates a copy of src under projectID with a new env key and
// re-seals every secret of src under it.
func (u *Unlocked) copyEnvTx(ctx context.Context, tx RecordStore, src domain.Environment, projectID domain.ID[domain.Project]) error {
	srcKey, err := u.envKey(ctx, tx, src.ID)
	if err != nil {
		return err
	}
	defer zeroKey(&srcKey)
	secrets, err := u.loadSecrets(ctx, tx, srcKey, src.ID)
	if err != nil {
		return err
	}
	dst := domain.Environment{ID: domain.NewID[domain.Environment](), ProjectID: projectID, Name: src.Name}
	dstKey, err := u.putEnvTx(ctx, tx, dst)
	if err != nil {
		return err
	}
	defer zeroKey(&dstKey)
	if err := u.recordHistoryTx(ctx, tx, dstKey, historyRec{HistoryEntry: HistoryEntry{EnvID: dst.ID, Op: OpEnvCreate}}); err != nil {
		return err
	}
	now := u.clock.Now().UTC()
	for _, s := range secrets {
		s.ID, s.EnvID, s.CreatedAt, s.UpdatedAt = domain.NewID[domain.Secret](), dst.ID, now, now
		if err := u.putSecretTx(ctx, tx, dstKey, s); err != nil {
			return err
		}
		if err := u.recordHistoryTx(ctx, tx, dstKey, historyRec{HistoryEntry: HistoryEntry{
			EnvID: dst.ID, Op: OpSecretCreate, SecretName: s.Name, NewValue: s.Value}}); err != nil {
			return err
		}
	}
	return nil
}
