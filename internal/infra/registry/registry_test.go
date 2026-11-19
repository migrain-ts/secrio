package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"secrio/internal/app"
	"secrio/internal/domain"
)

var _ app.Registry = (*Registry)(nil)

func TestAddListRemove(t *testing.T) {
	ctx := context.Background()
	r := New(filepath.Join(t.TempDir(), "cfg"))
	if got, err := r.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty list: %v %v", got, err)
	}
	e := app.VaultEntry{ID: "a", Name: "A", Path: "/p/a"}
	if err := r.Add(ctx, e); err != nil {
		t.Fatal(err)
	}
	got, err := r.List(ctx)
	if err != nil || len(got) != 1 || got[0] != e {
		t.Fatalf("list: %v %v", got, err)
	}
	if err := r.Remove(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.List(ctx); len(got) != 0 {
		t.Fatalf("after remove: %v", got)
	}
	if err := r.Remove(ctx, "a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAddDuplicateIDIsDuplicate(t *testing.T) {
	ctx := context.Background()
	r := New(t.TempDir())
	e := app.VaultEntry{ID: "a", Name: "A", Path: "/p"}
	if err := r.Add(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(ctx, e); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestConcurrentAddNoLoss(t *testing.T) {
	ctx := context.Background()
	r := New(t.TempDir())
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Add(ctx, app.VaultEntry{ID: fmt.Sprint(i), Name: "n", Path: "p"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got, err := r.List(ctx); err != nil || len(got) != n {
		t.Fatalf("got %d, %v", len(got), err)
	}
}

func TestMalformedFileIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vaults.json"), []byte("TOPSECRET{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := New(dir).List(context.Background())
	if !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
	if strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("error leaks content: %v", err)
	}
}

func TestFileModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	r := New(dir)
	if err := r.Add(context.Background(), app.VaultEntry{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	di, err := os.Stat(dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v %v", di.Mode(), err)
	}
	fi, err := os.Stat(filepath.Join(dir, "vaults.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v %v", fi.Mode(), err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("leftover temp files: %v", ents)
	}
}
