// Package keychain stores per-session keys in the OS keychain. It never
// stores a vault key: that lives sealed under the session key in the vault
// file.
package keychain

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"

	"secrio/internal/domain"
)

// KeyStore keeps one keychain item per vault: service=service, user=vaultID,
// holding the 32-byte session key in base64. It implements app.SessionKeys.
type KeyStore struct{ service string }

func New(service string) *KeyStore { return &KeyStore{service: service} }

// Save stores k. The intermediate buffer is zeroed; the string handed to the
// keyring API cannot be.
func (s *KeyStore) Save(_ context.Context, vaultID string, k domain.Key) error {
	buf := make([]byte, base64.StdEncoding.EncodedLen(len(k)))
	defer clear(buf)
	base64.StdEncoding.Encode(buf, k[:])
	if err := keyring.Set(s.service, vaultID, string(buf)); err != nil {
		return fmt.Errorf("keychain save: %w", err)
	}
	return nil
}

// Load returns domain.ErrLocked on a miss and domain.ErrCorrupt when the item
// is not a base64 32-byte key.
func (s *KeyStore) Load(_ context.Context, vaultID string) (domain.Key, error) {
	payload, err := keyring.Get(s.service, vaultID)
	if errors.Is(err, keyring.ErrNotFound) {
		return domain.Key{}, domain.ErrLocked
	}
	if err != nil {
		return domain.Key{}, fmt.Errorf("keychain load: %w", err)
	}
	return decode(payload)
}

// Delete is idempotent: a missing item is not an error.
func (s *KeyStore) Delete(_ context.Context, vaultID string) error {
	err := keyring.Delete(s.service, vaultID)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain delete: %w", err)
	}
	return nil
}

// decode parses a base64 session key. Errors never include the payload.
func decode(payload string) (domain.Key, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	defer clear(raw)
	var k domain.Key
	if err != nil || len(raw) != len(k) {
		return domain.Key{}, fmt.Errorf("session key undecodable: %w", domain.ErrCorrupt)
	}
	copy(k[:], raw)
	return k, nil
}

// MemoryKeyStore is an in-process app.SessionKeys. It is meant for tests and
// as a non-persistent fallback when no OS keychain is available.
type MemoryKeyStore struct {
	mu sync.Mutex
	m  map[string]domain.Key
}

func NewMemory() *MemoryKeyStore { return &MemoryKeyStore{m: map[string]domain.Key{}} }

func (s *MemoryKeyStore) Save(_ context.Context, vaultID string, k domain.Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[vaultID] = k
	return nil
}

func (s *MemoryKeyStore) Load(_ context.Context, vaultID string) (domain.Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.m[vaultID]
	if !ok {
		return domain.Key{}, domain.ErrLocked
	}
	return k, nil
}

func (s *MemoryKeyStore) Delete(_ context.Context, vaultID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, vaultID)
	return nil
}
