// Package core is the composition root: it wires the real adapters into an
// app.Service. CLI and Desktop call New and use only app and domain types.
package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"secrio/internal/app"
	"secrio/internal/domain"
	"secrio/internal/infra/clock"
	"secrio/internal/infra/crypto"
	"secrio/internal/infra/keychain"
	"secrio/internal/infra/registry"
	"secrio/internal/infra/sqlite"
)

// crypto.Suite is both the Cipher and the KDF port.
var (
	_ app.Cipher = crypto.Suite{}
	_ app.KDF    = crypto.Suite{}
)

type config struct {
	params domain.KDFParams
	keys   app.SessionKeys
	clock  app.Clock
}

// The keychain stores satisfy the session key port.
var (
	_ app.SessionKeys = (*keychain.KeyStore)(nil)
	_ app.SessionKeys = (*keychain.MemoryKeyStore)(nil)
)

// Option overrides a default of New.
type Option func(*config)

// WithKDFParams sets the Argon2id cost for newly created vaults.
func WithKDFParams(p domain.KDFParams) Option { return func(c *config) { c.params = p } }

// WithSessionKeys replaces the OS keychain session key store.
func WithSessionKeys(k app.SessionKeys) Option { return func(c *config) { c.keys = k } }

// WithClock replaces the system clock.
func WithClock(cl app.Clock) Option { return func(c *config) { c.clock = cl } }

// New builds a Service whose vault registry lives in dataDir (created with
// mode 0700 if missing).
func New(dataDir string, opts ...Option) (*app.Service, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	cfg := config{params: crypto.DefaultKDFParams(), keys: keychain.New("secrio"), clock: clock.System{}}
	for _, o := range opts {
		o(&cfg)
	}
	return app.NewService(crypto.Suite{}, crypto.Suite{}, cfg.params,
		sqlite.Opener{}, registry.New(dataDir), cfg.keys, cfg.clock), nil
}

// DefaultDataDir is <user config dir>/secrio.
func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "secrio"), nil
}

// VaultPath returns <dataDir>/vaults/<fileName>.db, creating the vaults
// directory (0700). fileName must be a plain name: empty names, path
// separators, "..", and NUL are rejected.
func VaultPath(dataDir, fileName string) (string, error) {
	if fileName == "" || strings.ContainsAny(fileName, "/\\\x00") || strings.Contains(fileName, "..") {
		return "", fmt.Errorf("%w: vault file name", domain.ErrInvalidName)
	}
	dir := filepath.Join(dataDir, "vaults")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.Join(errors.New("create vaults dir"), err)
	}
	return filepath.Join(dir, fileName+".db"), nil
}
