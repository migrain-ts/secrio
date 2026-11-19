package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// diskOpener makes fakeOpener.Create also leave real files on disk, like a
// SQLite vault with its -wal and -shm companions.
type diskOpener struct{ *fakeOpener }

func (o diskOpener) Create(ctx context.Context, path string) (RecordStore, error) {
	st, err := o.fakeOpener.Create(ctx, path)
	if err != nil {
		return nil, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(path+suffix, []byte("x"), 0o600); err != nil {
			return nil, err
		}
	}
	return st, nil
}

type failingRegistry struct{ fakeRegistry }

func (*failingRegistry) Add(context.Context, VaultEntry) error { return errors.New("registry down") }

func TestCreateVaultCleansUpWhenRegistryFails(t *testing.T) {
	h := newHarness(t)
	reg := &failingRegistry{}
	svc := NewService(newTestCipher(), newTestKDF(), testParams, diskOpener{h.opener}, reg, h.sessions, h.clock)
	path := filepath.Join(t.TempDir(), "v.db")
	_, err := svc.CreateVault(context.Background(), "n", path, []byte(testPw))
	if err == nil || err.Error() != "registry down" {
		t.Fatalf("got %v, want the original error", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, statErr := os.Stat(path + suffix); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%q left behind (%v)", path+suffix, statErr)
		}
	}
	// the store handle must be closed (harness cleanup checks h.opener.files)
}

func TestCreateVaultStoresAbsolutePath(t *testing.T) {
	h := newHarness(t)
	v, err := h.svc.CreateVault(context.Background(), "n", "rel/v.db", []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("rel/v.db")
	entries, _ := h.svc.ListVaults(context.Background())
	if len(entries) != 1 || entries[0].Path != want || entries[0].ID != string(v.ID) {
		t.Fatalf("entries = %+v, want path %q", entries, want)
	}
	if err := h.svc.Unlock(context.Background(), string(v.ID), []byte(testPw), time.Hour, 0); err != nil {
		t.Fatal(err)
	}
}
