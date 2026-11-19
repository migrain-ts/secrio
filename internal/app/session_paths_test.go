package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"secrio/internal/domain"
)

// hookOpener wraps the fake opener's stores with optional failure hooks.
type hookOpener struct {
	*fakeOpener
	deleteErr  error
	getMissing bool // GetSession reports ErrLocked although the row exists
}

type hookStore struct {
	RecordStore
	o *hookOpener
}

func (o *hookOpener) Open(ctx context.Context, path string) (RecordStore, error) {
	st, err := o.fakeOpener.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return &hookStore{RecordStore: st, o: o}, nil
}

func (s *hookStore) DeleteSession(ctx context.Context) error {
	if s.o.deleteErr != nil {
		return s.o.deleteErr
	}
	return s.RecordStore.DeleteSession(ctx)
}

func (s *hookStore) GetSession(ctx context.Context) (SessionRow, error) {
	if s.o.getMissing {
		return SessionRow{}, domain.ErrLocked
	}
	return s.RecordStore.GetSession(ctx)
}

func hookHarness(t *testing.T) (*harness, *hookOpener) {
	h := newHarness(t)
	o := &hookOpener{fakeOpener: h.opener}
	h.svc = NewService(newTestCipher(), newTestKDF(), testParams, o, h.reg, h.keys, h.clock)
	return h, o
}

func TestLockDeletesKeyWhenStoreCannotBeOpened(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	delete(h.opener.files, testPath) // registered, but the file is gone
	err := h.svc.Lock(context.Background(), id)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v, want the open error", err)
	}
	if _, ok := h.keys.m[id]; ok {
		t.Fatal("session key not deleted")
	}
}

func TestLockDeletesKeyWhenDeleteSessionFails(t *testing.T) {
	h, o := hookHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	boom := errors.New("disk full")
	o.deleteErr = boom
	if err := h.svc.Lock(context.Background(), id); !errors.Is(err, boom) {
		t.Fatalf("got %v, want DeleteSession error", err)
	}
	if _, ok := h.keys.m[id]; ok {
		t.Fatal("session key not deleted")
	}
	if h.vault().session == nil {
		t.Fatal("row unexpectedly gone")
	}
}

func TestOpenGetSessionMissAfterTouchIsLockedWithoutCleanup(t *testing.T) {
	h, o := hookHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	o.getMissing = true
	_, err := h.svc.Open(context.Background(), id)
	assertLocked(t, err)
	if _, ok := h.keys.m[id]; !ok {
		t.Fatal("key deleted although a concurrent Lock owns the cleanup")
	}
	if h.vault().session == nil {
		t.Fatal("row deleted")
	}
}

func TestOpenAndLockUnknownVaultID(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.keys.m["ghost"] = domain.Key{1}
	if _, err := h.svc.Open(ctx, "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := h.keys.m["ghost"]; !ok {
		t.Fatal("Open must not touch the key of an unknown vault")
	}
	if err := h.svc.Lock(ctx, "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Lock: %v", err)
	}
	if _, ok := h.keys.m["ghost"]; ok {
		t.Fatal("Lock must still delete the key under an unknown id")
	}
}
