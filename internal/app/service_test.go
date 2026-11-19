package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"secrio/internal/domain"
)

func TestCreateVaultThenUnlockOpen(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	v, err := h.svc.CreateVault(ctx, "personal", testPath, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "personal" || len(v.ID) != 32 || !v.CreatedAt.Equal(h.clock.Now()) {
		t.Fatalf("vault = %+v", v)
	}
	if meta := h.vault().meta; meta == nil || meta.ID != string(v.ID) || meta.Name != "personal" {
		t.Fatalf("meta = %+v", meta)
	}
	entries, err := h.svc.ListVaults(ctx)
	if err != nil || len(entries) != 1 || entries[0] != (VaultEntry{ID: string(v.ID), Name: "personal", Path: testPath}) {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	if err := h.svc.Unlock(ctx, string(v.ID), []byte(testPw), time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	u, err := h.svc.Open(ctx, string(v.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if _, err := u.CreateProject(ctx, "api", "backend"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.CreateVault(ctx, "again", testPath, []byte(testPw)); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("second create on same path = %v", err)
	}
}

func TestCreateVaultSeedsNothing(t *testing.T) {
	h := newHarness(t)
	u, _ := h.unlocked(t)
	ps, err := u.ListProjects(context.Background())
	if err != nil || len(ps) != 0 || len(h.vault().records) != 0 {
		t.Fatalf("projects = %v, %v", ps, err)
	}
	if len(DefaultEnvironments) != 4 || DefaultEnvironments[0] != "Development" || DefaultEnvironments[3] != "Production" {
		t.Fatalf("defaults = %v", DefaultEnvironments)
	}
}

func TestOpenWithoutUnlockIsLocked(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	if _, err := h.svc.Open(context.Background(), id); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("got %v", err)
	}
}

func TestUnknownVaultIsNotFound(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.newVault(t)
	if err := h.svc.Unlock(ctx, "nope", []byte(testPw), time.Hour, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unlock = %v", err)
	}
	if _, err := h.svc.OpenWithPassword(ctx, "nope", []byte(testPw)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("open = %v", err)
	}
	if err := h.svc.ChangePassword(ctx, "nope", []byte(testPw), []byte("x")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("change = %v", err)
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	if err := h.svc.Unlock(ctx, id, []byte("wrong"), time.Hour, 0); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("got %v", err)
	}
	if len(h.keys.m) != 0 || h.vault().session != nil {
		t.Fatal("session saved on wrong password")
	}
	if _, err := h.svc.OpenWithPassword(ctx, id, []byte("wrong")); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("open: %v", err)
	}
}

func TestOpenWithPasswordDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	u, err := h.svc.OpenWithPassword(ctx, id, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.CreateProject(ctx, "p", ""); err != nil {
		t.Fatal(err)
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	if len(h.keys.m) != 0 || h.vault().session != nil {
		t.Fatal("OpenWithPassword persisted a session")
	}
	if _, err := h.svc.Open(ctx, id); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("got %v", err)
	}
}

func TestCloseZeroesKeyAndClosesStore(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	u, err := h.svc.OpenWithPassword(context.Background(), id, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	if u.key == (domain.Key{}) || h.vault().handles != 1 {
		t.Fatalf("handle not open: handles=%d", h.vault().handles)
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	if u.key != (domain.Key{}) {
		t.Fatal("vault key not zeroed")
	}
	if h.vault().handles != 0 {
		t.Fatal("store not closed")
	}
	if err := u.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestChangePasswordKeepsSecretsReadable(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, id := h.unlocked(t)
	p, _ := u.CreateProject(ctx, "p", "")
	e, _ := u.CreateEnvironment(ctx, p.ID, "Development")
	if _, err := u.SetSecret(ctx, e.ID, "TOKEN", []byte("v1"), "", ""); err != nil {
		t.Fatal(err)
	}
	u.Close()

	h.vault().puts = map[Kind]int{}
	newPw := []byte("new password")
	if err := h.svc.ChangePassword(ctx, id, []byte("wrong"), newPw); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("wrong old pw: %v", err)
	}
	if err := h.svc.ChangePassword(ctx, id, []byte(testPw), newPw); err != nil {
		t.Fatal(err)
	}
	if n := h.vault().puts[KindSecret]; n != 0 {
		t.Fatalf("%d secret records rewritten", n)
	}
	if len(h.vault().puts) != 0 {
		t.Fatalf("records rewritten: %v", h.vault().puts)
	}

	u2, err := h.svc.Open(ctx, id) // existing session still works
	if err != nil {
		t.Fatal(err)
	}
	got, err := u2.GetSecret(ctx, e.ID, "TOKEN")
	u2.Close()
	if err != nil || string(got.Value) != "v1" {
		t.Fatalf("secret after change: %v", err)
	}
	if _, err := h.svc.OpenWithPassword(ctx, id, []byte(testPw)); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("old password: %v", err)
	}
	u3, err := h.svc.OpenWithPassword(ctx, id, newPw)
	if err != nil {
		t.Fatalf("new password: %v", err)
	}
	u3.Close()
}

func TestUnknownFormatVersionRefused(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.vault().meta.FormatVersion = 99
	if err := h.svc.Unlock(ctx, id, []byte(testPw), time.Hour, 0); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Unlock = %v", err)
	}
	if _, err := h.svc.OpenWithPassword(ctx, id, []byte(testPw)); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("OpenWithPassword = %v", err)
	}
	if err := h.svc.ChangePassword(ctx, id, []byte(testPw), []byte("new")); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("ChangePassword = %v", err)
	}
	if len(h.keys.m) != 0 || h.vault().session != nil {
		t.Fatal("session saved for unsupported format")
	}
}
