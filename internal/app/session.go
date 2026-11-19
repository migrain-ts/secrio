package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"secrio/internal/domain"
)

// Unlocked is an open vault with its vault key in memory. The caller must
// Close it. It is not safe for concurrent use, but after Close every method
// returns domain.ErrLocked, and Close itself may race with other calls (it
// waits for calls in flight).
type Unlocked struct {
	c     Cipher
	kdf   KDF
	clock Clock
	mu    sync.RWMutex // methods hold R; Close holds W
	store RecordStore  // nil once closed
	key   domain.Key   // vault key
}

// enter is the guard every exported method starts with: it fails with
// domain.ErrLocked after Close, otherwise returns the release func to defer.
// Methods must not call each other's exported forms (nested R locks).
func (u *Unlocked) enter() (func(), error) {
	u.mu.RLock()
	if u.store == nil {
		u.mu.RUnlock()
		return nil, domain.ErrLocked
	}
	return u.mu.RUnlock, nil
}

// Close zeroes the vault key and closes the store. It is idempotent.
func (u *Unlocked) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	zeroKey(&u.key)
	if u.store == nil {
		return nil
	}
	err := u.store.Close()
	u.store = nil
	return err
}

// Open returns a handle from the persisted session. A missing, expired
// (TTL) or idle session yields domain.ErrLocked; an expired one is deleted.
// Success refreshes the session's LastUsed.
//
// Load..Save is serialised with Lock and Unlock in this process only. A Lock
// issued by another process during that brief window can be undone, leaving
// the session valid until its TTL or idle expiry.
func (s *Service) Open(ctx context.Context, vaultID string) (*Unlocked, error) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	sess, err := s.sessions.Load(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	defer zeroKey(&sess.VaultKey)
	now := s.clock.Now()
	if !now.Before(sess.ExpiresAt) || (sess.Idle > 0 && now.Sub(sess.LastUsed) >= sess.Idle) {
		return nil, errors.Join(domain.ErrLocked, s.sessions.Delete(ctx, vaultID))
	}
	st, _, err := s.openStore(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	sess.LastUsed = now
	if err := s.sessions.Save(ctx, vaultID, sess); err != nil {
		return nil, errors.Join(err, st.Close())
	}
	return &Unlocked{c: s.c, kdf: s.kdf, clock: s.clock, store: st, key: sess.VaultKey}, nil
}

// OpenWithPassword returns a handle without persisting any session.
func (s *Service) OpenWithPassword(ctx context.Context, vaultID string, password []byte) (*Unlocked, error) {
	st, meta, err := s.openStore(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	vk, err := unwrapVaultKey(s.c, s.kdf, meta, password)
	if err != nil {
		return nil, errors.Join(err, st.Close())
	}
	return &Unlocked{c: s.c, kdf: s.kdf, clock: s.clock, store: st, key: vk}, nil
}

// --- sealed record helpers shared by structure.go and secrets.go ---

// putSealed seals v under k bound to (kind, id) and stores it.
func putSealed[T any](ctx context.Context, rs RecordStore, u *Unlocked, k domain.Key, kind Kind, id, parentID string, v T) error {
	blob, err := seal(u.c, k, kind, id, v)
	if err != nil {
		return err
	}
	return rs.Put(ctx, Record{Kind: kind, ID: id, ParentID: parentID, Blob: blob, UpdatedAt: u.clock.Now().UTC()})
}

// openRecord decrypts r under k. With the right key an authentication
// failure means the stored record is corrupt or tampered: domain.ErrCorrupt.
func openRecord[T any](c Cipher, k domain.Key, r Record) (T, error) {
	v, err := open[T](c, k, r.Kind, r.ID, r.Blob)
	if errors.Is(err, domain.ErrAuthFailed) {
		return v, fmt.Errorf("%w: %s record does not authenticate", domain.ErrCorrupt, r.Kind)
	}
	return v, err
}

// getRecord loads and decrypts one record; a miss is domain.ErrNotFound.
func getRecord[T any](ctx context.Context, rs RecordStore, c Cipher, k domain.Key, kind Kind, id string) (T, error) {
	var zero T
	r, err := rs.Get(ctx, kind, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return zero, fmt.Errorf("%s: %w", kind, domain.ErrNotFound)
		}
		return zero, err
	}
	return openRecord[T](c, k, r)
}

// listRecords loads and decrypts every record of kind under parentID.
func listRecords[T any](ctx context.Context, rs RecordStore, c Cipher, k domain.Key, kind Kind, parentID string) ([]T, error) {
	recs, err := rs.List(ctx, kind, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(recs))
	for _, r := range recs {
		v, err := openRecord[T](c, k, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
