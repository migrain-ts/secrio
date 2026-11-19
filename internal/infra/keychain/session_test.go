package keychain

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"secrio/internal/app"
	"secrio/internal/domain"
)

var (
	_ app.SessionKeys = (*KeyStore)(nil)
	_ app.SessionKeys = (*MemoryKeyStore)(nil)
)

func sampleKey() domain.Key {
	var k domain.Key
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func check(t *testing.T, name string, s app.SessionKeys) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Load(ctx, "v1"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("%s miss: want ErrLocked, got %v", name, err)
	}
	if err := s.Delete(ctx, "v1"); err != nil {
		t.Fatalf("%s delete missing: %v", name, err)
	}
	in := sampleKey()
	if err := s.Save(ctx, "v1", in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, "v2"); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("%s other vault: want ErrLocked, got %v", name, err)
	}
	out, err := s.Load(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("%s round trip mismatch", name)
	}
	if err := s.Delete(ctx, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "v1"); err != nil {
		t.Fatalf("%s second delete: %v", name, err)
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

func TestStoredItemIsOnlyTheBase64Key(t *testing.T) {
	keyring.MockInit()
	if err := New("svc").Save(context.Background(), "v", sampleKey()); err != nil {
		t.Fatal(err)
	}
	got, err := keyring.Get("svc", "v")
	if err != nil {
		t.Fatal(err)
	}
	k := sampleKey()
	if got != base64.StdEncoding.EncodeToString(k[:]) {
		t.Fatal("stored item is not exactly the base64 session key")
	}
}

func TestLoadCorruptPayload(t *testing.T) {
	keyring.MockInit()
	short := base64.StdEncoding.EncodeToString([]byte("SHORTKEYBYTES"))
	for _, payload := range []string{"SECRETJUNK!!", short, ""} {
		if err := keyring.Set("svc", "v", payload); err != nil {
			t.Fatal(err)
		}
		_, err := New("svc").Load(context.Background(), "v")
		if !errors.Is(err, domain.ErrCorrupt) {
			t.Fatalf("want ErrCorrupt, got %v", err)
		}
		if strings.Contains(err.Error(), "SECRETJUNK") || (strings.Contains(err.Error(), short)) {
			t.Fatalf("leak: %v", err)
		}
	}
}
