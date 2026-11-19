// Package keychain stores unlocked-vault sessions in the OS keychain.
package keychain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// SessionStore keeps one keychain item per vault: service=service, user=vaultID.
type SessionStore struct{ service string }

func New(service string) *SessionStore { return &SessionStore{service: service} }

// wire is the stored payload; the key is base64 (domain.Key would marshal as a number array).
type wire struct {
	Key       string `json:"key"`
	ExpiresAt string `json:"expires_at"`
	LastUsed  string `json:"last_used"`
	Idle      int64  `json:"idle"`
}

func (s *SessionStore) Save(_ context.Context, vaultID string, sess app.Session) error {
	raw := sess.VaultKey[:]
	w := wire{
		Key:       base64.StdEncoding.EncodeToString(raw),
		ExpiresAt: sess.ExpiresAt.UTC().Format(time.RFC3339Nano),
		LastUsed:  sess.LastUsed.UTC().Format(time.RFC3339Nano),
		Idle:      int64(sess.Idle),
	}
	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	defer zero(data)
	if err := keyring.Set(s.service, vaultID, string(data)); err != nil {
		return fmt.Errorf("keychain save: %w", err)
	}
	return nil
}

func (s *SessionStore) Load(_ context.Context, vaultID string) (app.Session, error) {
	payload, err := keyring.Get(s.service, vaultID)
	if errors.Is(err, keyring.ErrNotFound) {
		return app.Session{}, domain.ErrLocked
	}
	if err != nil {
		return app.Session{}, fmt.Errorf("keychain load: %w", err)
	}
	return decode(payload)
}

func (s *SessionStore) Delete(_ context.Context, vaultID string) error {
	err := keyring.Delete(s.service, vaultID)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain delete: %w", err)
	}
	return nil
}

func decode(payload string) (app.Session, error) {
	corrupt := fmt.Errorf("session payload undecodable: %w", domain.ErrCorrupt)
	var w wire
	if err := json.Unmarshal([]byte(payload), &w); err != nil {
		return app.Session{}, corrupt
	}
	raw, err := base64.StdEncoding.DecodeString(w.Key)
	if err != nil {
		return app.Session{}, corrupt
	}
	defer zero(raw)
	var key domain.Key
	if len(raw) != len(key) {
		return app.Session{}, corrupt
	}
	copy(key[:], raw)
	exp, err1 := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	last, err2 := time.Parse(time.RFC3339Nano, w.LastUsed)
	if err1 != nil || err2 != nil {
		return app.Session{}, corrupt
	}
	return app.Session{VaultKey: key, ExpiresAt: exp.UTC(), LastUsed: last.UTC(), Idle: time.Duration(w.Idle)}, nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// MemorySessionStore is an in-process app.SessionStore. It is meant for tests
// and as a non-persistent fallback when no OS keychain is available.
type MemorySessionStore struct {
	mu sync.Mutex
	m  map[string]app.Session
}

func NewMemory() *MemorySessionStore { return &MemorySessionStore{m: map[string]app.Session{}} }

func (s *MemorySessionStore) Save(_ context.Context, vaultID string, sess app.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[vaultID] = sess
	return nil
}

func (s *MemorySessionStore) Load(_ context.Context, vaultID string) (app.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[vaultID]
	if !ok {
		return app.Session{}, domain.ErrLocked
	}
	return sess, nil
}

func (s *MemorySessionStore) Delete(_ context.Context, vaultID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, vaultID)
	return nil
}
