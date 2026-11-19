package keychain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"secrio/internal/app"
	"secrio/internal/domain"
)

var (
	_ app.SessionStore = (*SessionStore)(nil)
	_ app.SessionStore = (*MemorySessionStore)(nil)
)

func sample() app.Session {
	var k domain.Key
	for i := range k {
		k[i] = byte(i + 1)
	}
	base := time.Date(2026, 11, 19, 10, 11, 12, 123456789, time.UTC)
	return app.Session{VaultKey: k, ExpiresAt: base.Add(time.Hour), LastUsed: base, Idle: 15*time.Minute + 7}
}

func check(t *testing.T, name string, s app.SessionStore) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Load(ctx, "v1"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("%s miss: want ErrLocked, got %v", name, err)
	}
	if err := s.Delete(ctx, "v1"); err != nil {
		t.Fatalf("%s delete missing: %v", name, err)
	}
	in := sample()
	if err := s.Save(ctx, "v1", in); err != nil {
		t.Fatal(err)
	}
	out, err := s.Load(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if out.VaultKey != in.VaultKey || !out.ExpiresAt.Equal(in.ExpiresAt) || !out.LastUsed.Equal(in.LastUsed) || out.Idle != in.Idle {
		t.Fatalf("%s round trip mismatch", name)
	}
	if out.LastUsed.Nanosecond() != 123456789 {
		t.Fatalf("%s lost nanoseconds", name)
	}
	if err := s.Delete(ctx, "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, "v1"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("%s after delete: %v", name, err)
	}
}

func TestSaveLoadDelete(t *testing.T) {
	keyring.MockInit()
	check(t, "keychain", New("secrio-test"))
}

func TestMemorySaveLoadDelete(t *testing.T) { check(t, "memory", NewMemory()) }

func TestLoadMissingIsLocked(t *testing.T) {
	keyring.MockInit()
	if _, err := New("secrio-test").Load(context.Background(), "nope"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCorruptPayload(t *testing.T) {
	keyring.MockInit()
	for _, payload := range []string{
		"SECRETJUNK",
		`{"key":"AAAA","expires_at":"2026-01-01T00:00:00Z","last_used":"2026-01-01T00:00:00Z","idle":1}`,
	} {
		if err := keyring.Set("svc", "v", payload); err != nil {
			t.Fatal(err)
		}
		_, err := New("svc").Load(context.Background(), "v")
		if !errors.Is(err, domain.ErrCorrupt) {
			t.Fatalf("want ErrCorrupt, got %v", err)
		}
		if strings.Contains(err.Error(), "SECRETJUNK") || strings.Contains(err.Error(), "AAAA") {
			t.Fatalf("leak: %v", err)
		}
	}
}
