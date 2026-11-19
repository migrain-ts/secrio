package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"secrio/internal/domain"
)

func ops(es []HistoryEntry) []HistoryOp {
	var o []HistoryOp
	for _, e := range es {
		o = append(o, e.Op)
	}
	return o
}

func TestSetSecretWritesHistory(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	h.clock.Advance(time.Minute)
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeText, "")
	h.clock.Advance(time.Minute)
	u.SetSecret(ctx, e.ID, "K", []byte("v2"), domain.TypeText, "")
	es, err := u.History(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []HistoryOp{"secret.update", "secret.create", "env.create"}
	if fmt.Sprint(ops(es)) != fmt.Sprint(want) {
		t.Fatalf("ops = %v, want %v", ops(es), want)
	}
	if es[0].SecretName != "K" || es[2].SecretName != "" || !es[0].At.After(es[1].At) {
		t.Fatalf("bad entries: %+v", es)
	}
}

func TestHistorySameTimestampOrder(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), "", "")
	u.SetSecret(ctx, e.ID, "K", []byte("v2"), "", "")
	es, _ := u.History(ctx, e.ID)
	if es[0].Op != "secret.update" || es[1].Op != "secret.create" {
		t.Fatalf("ops = %v", ops(es))
	}
}

func TestUnchangedSetWritesNoEvent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeText, "d")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeText, "d")
	es, _ := u.History(ctx, e.ID)
	if len(es) != 2 {
		t.Fatalf("got %v", ops(es))
	}
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeText, "d2") // description change
	es, _ = u.History(ctx, e.ID)
	if len(es) != 3 || es[0].Op != "secret.update" {
		t.Fatalf("got %v", ops(es))
	}
}

func TestHistoryListHasNoValues(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("s3cr3t-one"), "", "")
	u.SetSecret(ctx, e.ID, "K", []byte("s3cr3t-two"), "", "")
	u.DeleteSecret(ctx, e.ID, "K")
	es, err := u.History(ctx, e.ID)
	if err != nil || len(es) != 4 {
		t.Fatalf("%v %d", err, len(es))
	}
	for _, en := range es {
		if en.PrevValue != nil || en.NewValue != nil {
			t.Fatalf("values leaked: %s", en.Op)
		}
	}
	for _, r := range h.vault().records {
		if r.Kind == KindHistory && strings.Contains(string(r.Blob), "s3cr3t") {
			t.Fatal("plaintext in blob")
		}
	}
}

func TestHistoryEntryRedaction(t *testing.T) {
	en := HistoryEntry{ID: "x", Op: "secret.update", SecretName: "K", PrevValue: []byte("hunter2"), NewValue: []byte("hunter3")}
	for _, f := range []string{"%v", "%+v", "%#v", "%s", "%d", "%x"} {
		if got := fmt.Sprintf(f, en); strings.Contains(got, "hunter") || strings.Contains(got, "6875") {
			t.Fatalf("%s leaks: %s", f, got)
		}
	}
	if got := fmt.Sprintf("%v", &en); strings.Contains(got, "hunter") {
		t.Fatal(got)
	}
}

func TestDeleteEnvironmentDeletesItsHistory(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v"), "", "")
	if h.vault().countRecords(KindHistory) != 2 {
		t.Fatal("want 2 history records")
	}
	if err := u.DeleteEnvironment(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if n := h.vault().countRecords(KindHistory); n != 0 {
		t.Fatalf("%d history records left", n)
	}
}

func TestDeleteProjectDeletesItsHistory(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v"), "", "")
	if err := u.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if n := h.vault().countRecords(KindHistory); n != 0 {
		t.Fatalf("%d history records left", n)
	}
}

func TestDuplicateProjectRecordsHistory(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v"), "", "")
	cp, err := u.DuplicateProject(ctx, p.ID, "copy")
	if err != nil {
		t.Fatal(err)
	}
	envs, _ := u.ListEnvironments(ctx, cp.ID)
	es, err := u.History(ctx, envs[0].ID)
	if err != nil || fmt.Sprint(ops(es)) != fmt.Sprint([]HistoryOp{"secret.create", "env.create"}) {
		t.Fatalf("%v %v", err, ops(es))
	}
}

func TestRestoreRequiresPassword(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), "", "")
	u.SetSecret(ctx, e.ID, "K", []byte("v2"), "", "")
	es, _ := u.History(ctx, e.ID)
	err := u.RestoreVersion(ctx, es[0].ID, []byte("wrong"))
	if !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("err = %v", err)
	}
	s, _ := u.GetSecret(ctx, e.ID, "K")
	if string(s.Value) != "v2" {
		t.Fatalf("value changed: %q", s.Value)
	}
	if es2, _ := u.History(ctx, e.ID); len(es2) != len(es) {
		t.Fatal("history changed")
	}
}

func TestRestoreRestoresPreviousValue(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeToken, "old")
	u.SetSecret(ctx, e.ID, "K", []byte("v2"), domain.TypeText, "new")
	es, _ := u.History(ctx, e.ID)
	if err := u.RestoreVersion(ctx, es[0].ID, []byte(testPw)); err != nil {
		t.Fatal(err)
	}
	s, _ := u.GetSecret(ctx, e.ID, "K")
	if string(s.Value) != "v1" || s.Type != domain.TypeToken || s.Description != "old" {
		t.Fatalf("got %q %s %q", s.Value, s.Type, s.Description)
	}
	es, _ = u.History(ctx, e.ID)
	if es[0].Op != "secret.update" || len(es) != 4 {
		t.Fatalf("ops = %v", ops(es))
	}
}

func TestRestoreDeletedSecretRecreatesIt(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), domain.TypeAPIKey, "d")
	u.DeleteSecret(ctx, e.ID, "K")
	es, _ := u.History(ctx, e.ID)
	if es[0].Op != "secret.delete" {
		t.Fatalf("ops = %v", ops(es))
	}
	if err := u.RestoreVersion(ctx, es[0].ID, []byte(testPw)); err != nil {
		t.Fatal(err)
	}
	s, err := u.GetSecret(ctx, e.ID, "K")
	if err != nil || string(s.Value) != "v1" || s.Type != domain.TypeAPIKey {
		t.Fatalf("%v %s", err, s.Type)
	}
	es, _ = u.History(ctx, e.ID)
	if es[0].Op != "secret.create" {
		t.Fatalf("ops = %v", ops(es))
	}
}

func TestRestoreNothingToRestore(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), "", "")
	es, _ := u.History(ctx, e.ID)
	for _, en := range es { // secret.create + env.create
		if err := u.RestoreVersion(ctx, en.ID, []byte(testPw)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s: err = %v", en.Op, err)
		}
	}
	if err := u.RestoreVersion(ctx, "nope", []byte(testPw)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// failHistoryStore fails Put of history records inside transactions.
type failHistoryStore struct{ RecordStore }

func (s failHistoryStore) Tx(ctx context.Context, fn func(RecordStore) error) error {
	return s.RecordStore.Tx(ctx, func(tx RecordStore) error { return fn(failHistoryTx{tx}) })
}

type failHistoryTx struct{ RecordStore }

func (s failHistoryTx) Put(ctx context.Context, r Record) error {
	if r.Kind == KindHistory {
		return errors.New("disk full")
	}
	return s.RecordStore.Put(ctx, r)
}

func TestHistoryAtomicWithWrite(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v1"), "", "")
	real := u.store
	u.store = failHistoryStore{real}
	if _, err := u.SetSecret(ctx, e.ID, "N", []byte("x"), "", ""); err == nil {
		t.Fatal("want error")
	}
	if _, err := u.SetSecret(ctx, e.ID, "K", []byte("v2"), "", ""); err == nil {
		t.Fatal("want error")
	}
	if err := u.DeleteSecret(ctx, e.ID, "K"); err == nil {
		t.Fatal("want error")
	}
	if _, err := u.CreateEnvironment(ctx, p.ID, "Prod"); err == nil {
		t.Fatal("want error")
	}
	u.store = real
	ss, _ := u.ListSecrets(ctx, e.ID)
	if len(ss) != 1 || string(ss[0].Value) != "v1" {
		t.Fatalf("secrets changed: %d", len(ss))
	}
	if envs, _ := u.ListEnvironments(ctx, p.ID); len(envs) != 1 {
		t.Fatalf("%d envs", len(envs))
	}
}

func TestHistoryBlobBoundToEnvKey(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e1 := newEnv(t, u, "p", "Dev")
	e2, _ := u.CreateEnvironment(ctx, p.ID, "Prod")
	u.SetSecret(ctx, e1.ID, "K", []byte("v"), "", "")
	var hr Record
	for _, r := range h.vault().records {
		if r.Kind == KindHistory && r.ParentID == string(e1.ID) {
			hr = r
		}
	}
	k2, err := u.envKey(ctx, u.store, e2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openRecord[historyRec](u.c, k2, hr); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("err = %v", err)
	}
}

func TestCorruptHistoryBlob(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	u.SetSecret(ctx, e.ID, "K", []byte("v"), "", "")
	for k, r := range h.vault().records {
		if r.Kind == KindHistory {
			r.Blob[len(r.Blob)-1] ^= 1
			h.vault().records[k] = r
		}
	}
	if _, err := u.History(ctx, e.ID); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("err = %v", err)
	}
}
