package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"secrio/internal/app/importexport"
	"secrio/internal/domain"
)

func ents(kv ...string) []importexport.Entry {
	var out []importexport.Entry
	for i := 0; i < len(kv); i += 2 {
		out = append(out, importexport.Entry{Name: domain.SecretName(kv[i]), Value: []byte(kv[i+1])})
	}
	return out
}

func names(ns []domain.SecretName) string {
	var s []string
	for _, n := range ns {
		s = append(s, string(n))
	}
	return strings.Join(s, ",")
}

func TestPlanImportFlagsDuplicates(t *testing.T) {
	ctx := context.Background()
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	if _, err := u.SetSecret(ctx, e.ID, "OLD", []byte("x"), domain.TypeText, ""); err != nil {
		t.Fatal(err)
	}
	plan, err := u.PlanImport(ctx, e.ID, ents("NEW1", "a", "OLD", "b", "NEW2", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if names(plan.New) != "NEW1,NEW2" || names(plan.Duplicate) != "OLD" {
		t.Fatalf("new=%s dup=%s", names(plan.New), names(plan.Duplicate))
	}
	if _, err := u.PlanImport(ctx, domain.NewID[domain.Environment](), nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown env: %v", err)
	}
	_, err = u.PlanImport(ctx, e.ID, ents("bad-name", "SECRETVAL"))
	if !errors.Is(err, domain.ErrInvalidName) || strings.Contains(err.Error(), "SECRETVAL") || strings.Contains(err.Error(), "bad-name") {
		t.Fatalf("invalid name: %v", err)
	}
}

func TestImportPlanRedacted(t *testing.T) {
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	plan, err := u.PlanImport(context.Background(), e.ID, ents("A", "topsecretvalue"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{fmt.Sprintf("%v", plan), fmt.Sprintf("%+v", plan), fmt.Sprintf("%#v", plan), fmt.Sprintf("%s", plan), fmt.Sprintf("%x", plan), fmt.Sprintf("%d", plan)} {
		if strings.Contains(s, "topsecret") || s != "ImportPlan{<redacted>}" {
			t.Fatalf("not redacted: %q", s)
		}
	}
}

func TestApplyImportSkipVsOverwrite(t *testing.T) {
	ctx := context.Background()
	for _, strat := range []Strategy{StrategySkip, StrategyOverwrite} {
		t.Run(string(strat), func(t *testing.T) {
			u, _ := newHarness(t).unlocked(t)
			_, e := newEnv(t, u, "p", "Dev")
			if _, err := u.SetSecret(ctx, e.ID, "OLD", []byte("old"), domain.TypePassword, "desc"); err != nil {
				t.Fatal(err)
			}
			if _, err := u.SetSecret(ctx, e.ID, "SAME", []byte("same"), domain.TypeText, ""); err != nil {
				t.Fatal(err)
			}
			plan, err := u.PlanImport(ctx, e.ID, ents("NEW", "n", "OLD", "changed", "SAME", "same"))
			if err != nil {
				t.Fatal(err)
			}
			before, _ := u.History(ctx, e.ID)
			if err := u.ApplyImport(ctx, e.ID, plan, strat); err != nil {
				t.Fatal(err)
			}
			get := func(n domain.SecretName) domain.Secret {
				s, err := u.GetSecret(ctx, e.ID, n)
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			if n := get("NEW"); string(n.Value) != "n" || n.Type != domain.TypeText || n.Description != "" {
				t.Fatalf("NEW wrong")
			}
			after, _ := u.History(ctx, e.ID)
			added := len(after) - len(before)
			if strat == StrategySkip {
				if string(get("OLD").Value) != "old" || get("OLD").Type != domain.TypePassword || added != 1 {
					t.Fatalf("skip touched duplicate (added %d history)", added)
				}
			} else {
				// NEW create + OLD update; SAME unchanged -> no event.
				if string(get("OLD").Value) != "changed" || added != 2 {
					t.Fatalf("overwrite wrong (added %d history)", added)
				}
				if after[0].Op != OpSecretUpdate && after[1].Op != OpSecretUpdate {
					t.Fatal("no secret.update recorded")
				}
			}
		})
	}
}

func TestApplyImportWritesHistory(t *testing.T) {
	ctx := context.Background()
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	plan, _ := u.PlanImport(ctx, e.ID, ents("A", "1", "B", "2"))
	if err := u.ApplyImport(ctx, e.ID, plan, StrategySkip); err != nil {
		t.Fatal(err)
	}
	hist, _ := u.History(ctx, e.ID)
	creates := 0
	for _, h := range hist {
		if h.Op == OpSecretCreate {
			creates++
		}
	}
	if creates != 2 {
		t.Fatalf("want 2 secret.create, got %d", creates)
	}
}

func TestApplyImportUnknownStrategyAndEnv(t *testing.T) {
	ctx := context.Background()
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	plan, _ := u.PlanImport(ctx, e.ID, ents("A", "1"))
	if err := u.ApplyImport(ctx, e.ID, plan, "merge"); err == nil || errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("want plain error, got %v", err)
	}
	if got, _ := u.ListSecrets(ctx, e.ID); len(got) != 0 {
		t.Fatal("imported despite bad strategy")
	}
	if err := u.ApplyImport(ctx, domain.NewID[domain.Environment](), plan, StrategySkip); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown env: %v", err)
	}
}

// failPutStore fails the Nth Put of a secret record inside a transaction.
type failPutStore struct {
	RecordStore
	n *int
}

func (f failPutStore) Tx(ctx context.Context, fn func(RecordStore) error) error {
	return f.RecordStore.Tx(ctx, func(tx RecordStore) error { return fn(failPutTx{tx, f.n}) })
}

type failPutTx struct {
	RecordStore
	n *int
}

func (f failPutTx) Put(ctx context.Context, r Record) error {
	if r.Kind == KindSecret {
		*f.n--
		if *f.n == 0 {
			return errors.New("injected put failure")
		}
	}
	return f.RecordStore.Put(ctx, r)
}

func TestApplyImportAtomicRollback(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	if _, err := u.SetSecret(ctx, e.ID, "OLD", []byte("old"), domain.TypeText, ""); err != nil {
		t.Fatal(err)
	}
	plan, err := u.PlanImport(ctx, e.ID, ents("A", "1", "OLD", "new", "B", "2", "C", "3"))
	if err != nil {
		t.Fatal(err)
	}
	histBefore := h.vault().countRecords(KindHistory)
	n := 3
	real := u.store
	u.store = failPutStore{real, &n}
	err = u.ApplyImport(ctx, e.ID, plan, StrategyOverwrite)
	u.store = real
	if err == nil {
		t.Fatal("want failure")
	}
	secrets, _ := u.ListSecrets(ctx, e.ID)
	if len(secrets) != 1 || string(secrets[0].Value) != "old" {
		t.Fatalf("not rolled back: %d secrets", len(secrets))
	}
	if got := h.vault().countRecords(KindHistory); got != histBefore {
		t.Fatalf("history leaked: %d -> %d", histBefore, got)
	}
}

func TestExportSelectedNamesOnly(t *testing.T) {
	ctx := context.Background()
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	for _, kv := range [][2]string{{"A", "1"}, {"B", "two words"}, {"C", "3"}} {
		if _, err := u.SetSecret(ctx, e.ID, domain.SecretName(kv[0]), []byte(kv[1]), domain.TypeText, ""); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := u.Export(ctx, e.ID, []domain.SecretName{"C", "B"}, importexport.FormatDotenv, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "B=\"two words\"\nC=3\n" {
		t.Fatalf("got %q", buf.String())
	}
	buf.Reset()
	if err := u.Export(ctx, e.ID, nil, importexport.FormatJSON, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"A"`) || !strings.Contains(buf.String(), `"C"`) {
		t.Fatalf("all: %q", buf.String())
	}
	buf.Reset()
	err := u.Export(ctx, e.ID, []domain.SecretName{"A", "MISSING"}, importexport.FormatYAML, &buf)
	if !errors.Is(err, domain.ErrNotFound) || strings.Contains(err.Error(), "MISSING") || buf.Len() != 0 {
		t.Fatalf("unknown name: %v (written %d)", err, buf.Len())
	}
	if err := u.Export(ctx, domain.NewID[domain.Environment](), nil, importexport.FormatYAML, &buf); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown env: %v", err)
	}
}
