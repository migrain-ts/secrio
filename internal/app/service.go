package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"secrio/internal/domain"
)

// DefaultEnvironments are the environment names interfaces offer by default.
// CreateVault seeds nothing.
var DefaultEnvironments = []string{"Development", "Testing", "Staging", "Production"}

// Service holds the vault-level use cases (create, unlock, lock, open,
// change password). Per-vault data operations live on *Unlocked.
type Service struct {
	c        Cipher
	kdf      KDF
	params   domain.KDFParams // KDF cost for new vaults
	opener   VaultOpener
	reg      Registry
	sessions SessionStore
	clock    Clock

	// sessionMu serialises session writes (Open's Load..Save, Lock, Unlock)
	// so a Lock cannot be undone by a concurrent Open in this process. It is
	// never held while calling back into the caller.
	sessionMu sync.Mutex
}

func NewService(c Cipher, kdf KDF, p domain.KDFParams, opener VaultOpener, reg Registry, sessions SessionStore, clock Clock) *Service {
	return &Service{c: c, kdf: kdf, params: p, opener: opener, reg: reg, sessions: sessions, clock: clock}
}

// closeInto closes st and joins any close error into *err.
func closeInto(err *error, st RecordStore) { *err = errors.Join(*err, st.Close()) }

// validName rejects blank display names (projects, environments, vaults).
func validName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: empty", domain.ErrInvalidName)
	}
	return nil
}

// CreateVault creates a new vault file at path and registers it.
func (s *Service) CreateVault(ctx context.Context, name, path string, password []byte) (_ domain.Vault, err error) {
	if err := validName(name); err != nil {
		return domain.Vault{}, err
	}
	if len(password) == 0 {
		return domain.Vault{}, fmt.Errorf("%w: empty", domain.ErrInvalidPassword)
	}
	meta, vk, err := newVaultKeys(s.c, s.kdf, s.params, password)
	if err != nil {
		return domain.Vault{}, err
	}
	zeroKey(&vk) // the vault key is only needed wrapped
	meta.Name = name
	meta.CreatedAt = s.clock.Now().UTC()

	st, err := s.opener.Create(ctx, path)
	if err != nil {
		return domain.Vault{}, err
	}
	defer closeInto(&err, st)
	if err := st.PutMeta(ctx, meta); err != nil {
		return domain.Vault{}, err
	}
	if err := s.reg.Add(ctx, VaultEntry{ID: meta.ID, Name: name, Path: path}); err != nil {
		return domain.Vault{}, err
	}
	return domain.Vault{ID: domain.ID[domain.Vault](meta.ID), Name: name, CreatedAt: meta.CreatedAt}, nil
}

// ListVaults returns the registered vaults. It needs no unlock.
func (s *Service) ListVaults(ctx context.Context) ([]VaultEntry, error) {
	return s.reg.List(ctx)
}

// openStore opens the registered vault file and checks its header matches
// vaultID. The caller must Close the store.
func (s *Service) openStore(ctx context.Context, vaultID string) (RecordStore, VaultMeta, error) {
	entries, err := s.reg.List(ctx)
	if err != nil {
		return nil, VaultMeta{}, err
	}
	path := ""
	for _, e := range entries {
		if e.ID == vaultID {
			path = e.Path
			break
		}
	}
	if path == "" {
		return nil, VaultMeta{}, fmt.Errorf("vault: %w", domain.ErrNotFound)
	}
	st, err := s.opener.Open(ctx, path)
	if err != nil {
		return nil, VaultMeta{}, err
	}
	meta, err := st.Meta(ctx)
	if err == nil && meta.ID != vaultID {
		err = fmt.Errorf("%w: vault id does not match registry", domain.ErrCorrupt)
	}
	if err != nil {
		return nil, VaultMeta{}, errors.Join(err, st.Close())
	}
	return st, meta, nil
}

// Unlock verifies password and persists a session valid for ttl, ending
// early after idle without use (idle <= 0 disables the idle timeout).
func (s *Service) Unlock(ctx context.Context, vaultID string, password []byte, ttl, idle time.Duration) (err error) {
	if ttl <= 0 {
		return errors.New("session ttl must be positive")
	}
	st, meta, err := s.openStore(ctx, vaultID)
	if err != nil {
		return err
	}
	defer closeInto(&err, st)
	vk, err := unwrapVaultKey(s.c, s.kdf, meta, password)
	if err != nil {
		return err
	}
	defer zeroKey(&vk)
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	now := s.clock.Now()
	return s.sessions.Save(ctx, vaultID, Session{VaultKey: vk, ExpiresAt: now.Add(ttl), LastUsed: now, Idle: idle})
}

// Lock forgets the persisted session of vaultID.
func (s *Service) Lock(ctx context.Context, vaultID string) error {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.sessions.Delete(ctx, vaultID)
}

// ChangePassword re-wraps the vault key under newPw. No data record is
// rewritten and existing sessions stay valid.
func (s *Service) ChangePassword(ctx context.Context, vaultID string, oldPw, newPw []byte) (err error) {
	if len(newPw) == 0 {
		return fmt.Errorf("%w: empty", domain.ErrInvalidPassword)
	}
	st, meta, err := s.openStore(ctx, vaultID)
	if err != nil {
		return err
	}
	defer closeInto(&err, st)
	nm, err := rewrapVaultKey(s.c, s.kdf, meta, oldPw, newPw)
	if err != nil {
		return err
	}
	return st.PutMeta(ctx, nm)
}
