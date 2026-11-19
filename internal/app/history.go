package app

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"secrio/internal/domain"
)

// History records: Kind=KindHistory, ID=random id, ParentID=env id, sealed
// under the ENV key. The sealed blob holds the secret values; they never
// leave the package through History. A deleted environment's history is
// deleted with it (deleteEnvTx), so there is no "env.delete" event.

type HistoryOp string

const (
	OpSecretCreate HistoryOp = "secret.create"
	OpSecretUpdate HistoryOp = "secret.update"
	OpSecretDelete HistoryOp = "secret.delete"
	OpEnvCreate    HistoryOp = "env.create"
)

// HistoryEntry is one recorded change. PrevValue and NewValue are always nil
// in entries returned by History.
type HistoryEntry struct {
	ID         string
	EnvID      domain.ID[domain.Environment]
	Op         HistoryOp
	SecretName domain.SecretName
	At         time.Time
	PrevValue  []byte
	NewValue   []byte
}

const redactedEntry = "HistoryEntry{<redacted>}"

func (HistoryEntry) String() string   { return redactedEntry }
func (HistoryEntry) GoString() string { return redactedEntry }

// Format redacts the entry for every fmt verb.
func (HistoryEntry) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redactedEntry)) }

// historyRec is the sealed form: the entry plus an ordering sequence and the
// previous type/description (needed to restore a version faithfully).
type historyRec struct {
	HistoryEntry
	Seq      int
	PrevType domain.SecretType
	PrevDesc string
}

// recordHistoryTx seals and stores one event for envID in tx.
func (u *Unlocked) recordHistoryTx(ctx context.Context, tx RecordStore, envKey domain.Key, rec historyRec) error {
	existing, err := tx.List(ctx, KindHistory, string(rec.EnvID))
	if err != nil {
		return err
	}
	rec.ID = string(domain.NewID[domain.Secret]())
	rec.Seq = len(existing) + 1
	rec.At = u.clock.Now().UTC()
	return putSealed(ctx, tx, u, envKey, KindHistory, rec.ID, string(rec.EnvID), rec)
}

// History returns the environment's events, newest first, without values.
func (u *Unlocked) History(ctx context.Context, envID domain.ID[domain.Environment]) ([]HistoryEntry, error) {
	done, lerr := u.enter()
	if lerr != nil {
		return nil, lerr
	}
	defer done()
	key, err := u.envKey(ctx, u.store, envID)
	if err != nil {
		return nil, err
	}
	defer zeroKey(&key)
	recs, err := listRecords[historyRec](ctx, u.store, u.c, key, KindHistory, string(envID))
	if err != nil {
		return nil, err
	}
	// Seq is strictly increasing per environment; At can tie or go backwards.
	sort.Slice(recs, func(i, j int) bool { return recs[i].Seq > recs[j].Seq })
	out := make([]HistoryEntry, len(recs))
	for i, r := range recs {
		out[i] = r.HistoryEntry
		out[i].PrevValue, out[i].NewValue = nil, nil
	}
	return out, nil
}

// RestoreVersion re-applies the previous value of a secret.update or
// secret.delete entry, after re-verifying the vault password. The restore is
// itself recorded as a normal event.
func (u *Unlocked) RestoreVersion(ctx context.Context, entryID string, password []byte) error {
	done, lerr := u.enter()
	if lerr != nil {
		return lerr
	}
	defer done()
	meta, err := u.store.Meta(ctx)
	if err != nil {
		return err
	}
	vk, err := unwrapVaultKey(u.c, u.kdf, meta, password)
	zeroKey(&vk)
	if err != nil {
		return err
	}
	return u.store.Tx(ctx, func(tx RecordStore) error {
		r, err := tx.Get(ctx, KindHistory, entryID)
		if err != nil {
			return err
		}
		envID := domain.ID[domain.Environment](r.ParentID)
		key, err := u.envKey(ctx, tx, envID)
		if err != nil {
			return err
		}
		defer zeroKey(&key)
		rec, err := openRecord[historyRec](u.c, key, r)
		if err != nil {
			return err
		}
		if rec.Op != OpSecretUpdate && rec.Op != OpSecretDelete {
			return fmt.Errorf("%w: nothing to restore", domain.ErrNotFound)
		}
		_, err = u.setSecretTx(ctx, tx, key, envID, rec.SecretName, rec.PrevValue, rec.PrevType, rec.PrevDesc)
		return err
	})
}

// setSecretTx upserts a secret by name and records the matching history
// event (none when nothing changed).
func (u *Unlocked) setSecretTx(ctx context.Context, tx RecordStore, key domain.Key, envID domain.ID[domain.Environment],
	name domain.SecretName, value []byte, typ domain.SecretType, desc string) (domain.Secret, error) {
	now := u.clock.Now().UTC()
	s := domain.Secret{ID: domain.NewID[domain.Secret](), EnvID: envID, Name: name, Value: bytes.Clone(value),
		Description: desc, Type: typ, CreatedAt: now, UpdatedAt: now}
	existing, err := u.loadSecrets(ctx, tx, key, envID)
	if err != nil {
		return domain.Secret{}, err
	}
	rec := historyRec{HistoryEntry: HistoryEntry{EnvID: envID, Op: OpSecretCreate, SecretName: name, NewValue: s.Value}}
	changed := true
	if old, ok := findSecret(existing, name); ok {
		s.ID, s.CreatedAt = old.ID, old.CreatedAt
		rec.Op, rec.PrevValue, rec.PrevType, rec.PrevDesc = OpSecretUpdate, old.Value, old.Type, old.Description
		changed = !bytes.Equal(old.Value, value) || old.Type != typ || old.Description != desc
	}
	if err := u.putSecretTx(ctx, tx, key, s); err != nil {
		return domain.Secret{}, err
	}
	if changed {
		if err := u.recordHistoryTx(ctx, tx, key, rec); err != nil {
			return domain.Secret{}, err
		}
	}
	return s, nil
}
