package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"secrio/internal/domain"
)

// DefaultEnvironments are the environment names interfaces offer by default.
// CreateVault seeds nothing.
var DefaultEnvironments = []string{"Development", "Testing", "Staging", "Production"}

// Service holds the vault-level use cases (create, unlock, lock, open,
// change password). Per-vault data operations live on *Unlocked.
type Service struct {
	c      Cipher
	kdf    KDF
	params domain.KDFParams // KDF cost for new vaults
	opener VaultOpener
	reg    Registry
	keys   SessionKeys
	clock  Clock
}

func NewService(c Cipher, kdf KDF, p domain.KDFParams, opener VaultOpener, reg Registry, keys SessionKeys, clock Clock) *Service {
	return &Service{c: c, kdf: kdf, params: p, opener: opener, reg: reg, keys: keys, clock: clock}
}

// closeInto closes st and joins any close error into *err.
func closeInto(err *error, st RecordStore) { *err = errors.Join(*err, st.Close()) }

// validName rejects blank display names and names with control characters
// (newline, ESC, ...) for projects, environments and vaults.
func validName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: empty", domain.ErrInvalidName)
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: control characters are not allowed", domain.ErrInvalidName)
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

	abs, err := filepath.Abs(path)
	if err != nil {
		return domain.Vault{}, errors.New("resolve vault path")
	}
	st, err := s.opener.Create(ctx, abs)
	if err != nil {
		return domain.Vault{}, err
	}
	registered := false
	defer func() {
		if registered {
			closeInto(&err, st)
			return
		}
		// Creation failed after the file exists: leave nothing behind.
		_ = st.Close()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(abs + suffix) // best effort; not-exist is fine
		}
	}()
	if err := st.PutMeta(ctx, meta); err != nil {
		return domain.Vault{}, err
	}
	if err := s.reg.Add(ctx, VaultEntry{ID: meta.ID, Name: name, Path: abs}); err != nil {
		return domain.Vault{}, err
	}
	registered = true
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
// early after idle without use (idle == 0 disables the idle timeout; ttl <= 0 or idle < 0 is
// domain.ErrInvalidArgument).
//
// A fresh random session key goes to SessionKeys; the vault key is stored
// sealed under it in the vault file's session row. The keychain never holds
// the vault key.
func (s *Service) Unlock(ctx context.Context, vaultID string, password []byte, ttl, idle time.Duration) (err error) {
	if ttl <= 0 {
		return fmt.Errorf("%w: session ttl must be positive", domain.ErrInvalidArgument)
	}
	if idle < 0 {
		return fmt.Errorf("%w: session idle must not be negative", domain.ErrInvalidArgument)
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
	sk, err := s.c.NewKey()
	if err != nil {
		return fmt.Errorf("session key: %w", err)
	}
	defer zeroKey(&sk)
	now := s.clock.Now()
	expires := now.Add(ttl)
	blob, err := s.c.Seal(sk, vk[:], sessionAAD(vaultID, expires, idle))
	if err != nil {
		return fmt.Errorf("seal session: %w", err)
	}
	if err := st.PutSession(ctx, SessionRow{Blob: blob, ExpiresAt: expires, LastUsed: now, Idle: idle}); err != nil {
		return err
	}
	if err := s.keys.Save(ctx, vaultID, sk); err != nil {
		return errors.Join(err, st.DeleteSession(ctx)) // no row without its key
	}
	return nil
}

// Lock ends the persisted session of vaultID: the session row first (it is
// the credential), then the session key. It is idempotent. If the vault file
// cannot be opened, the session key is still deleted.
func (s *Service) Lock(ctx context.Context, vaultID string) (err error) {
	st, _, err := s.openStore(ctx, vaultID)
	if err != nil {
		return errors.Join(err, s.keys.Delete(ctx, vaultID))
	}
	defer closeInto(&err, st)
	return s.killSession(ctx, st, vaultID)
}

// killSession deletes both halves of a session: row first, then key.
func (s *Service) killSession(ctx context.Context, st RecordStore, vaultID string) error {
	return errors.Join(st.DeleteSession(ctx), s.keys.Delete(ctx, vaultID))
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
