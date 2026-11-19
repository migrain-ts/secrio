package sqlite_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
	"secrio/internal/infra/sqlite"
)

func newStore(t *testing.T) (app.RecordStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.db")
	s, err := sqlite.Opener{}.Create(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.(io.Closer).Close() })
	return s, path
}

func rec(id, parent string) app.Record {
	return app.Record{Kind: app.KindSecret, ID: id, ParentID: parent, Blob: []byte("blob-" + id),
		UpdatedAt: time.UnixMilli(1700000000123)}
}

func TestCreateSetsMode0600(t *testing.T) {
	s, path := newStore(t)
	if err := s.Put(context.Background(), rec("a", "p")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		fi, err := os.Stat(p)
		if err != nil {
			if p == path {
				t.Fatal(err)
			}
			continue
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o", p, fi.Mode().Perm())
		}
	}
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Errorf("expected -wal file after write: %v", err)
	}
	if _, err := (sqlite.Opener{}).Create(context.Background(), path); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("second create = %v", err)
	}
}

func TestPutGetListDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	for _, r := range []app.Record{rec("a", "p"), rec("b", "p"), rec("c", "q")} {
		if err := s.Put(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get(ctx, app.KindSecret, "a")
	if err != nil || got.ParentID != "p" || string(got.Blob) != "blob-a" || !got.UpdatedAt.Equal(time.UnixMilli(1700000000123)) {
		t.Fatalf("get = %+v %v", got, err)
	}
	r := rec("a", "p")
	r.Blob = []byte("new")
	if err := s.Put(ctx, r); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(ctx, app.KindSecret, "a"); string(got.Blob) != "new" {
		t.Errorf("upsert failed: %s", got.Blob)
	}
	l, err := s.List(ctx, app.KindSecret, "p")
	if err != nil || len(l) != 2 {
		t.Fatalf("list = %v %v", l, err)
	}
	if err := s.Delete(ctx, app.KindSecret, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, app.KindSecret, "a"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after delete = %v", err)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Get(context.Background(), app.KindSecret, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if _, err := s.Meta(context.Background()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("meta err = %v", err)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	m := app.VaultMeta{FormatVersion: 1, ID: "vid", Name: "n",
		KDF:  domain.KDFParams{Time: 3, MemoryKiB: 65536, Threads: 4},
		Salt: []byte{1, 2}, WrappedVaultKey: []byte{3, 4}, CreatedAt: time.UnixMilli(1700000000123)}
	if err := s.PutMeta(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.Name = "n2"
	if err := s.PutMeta(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := s.Meta(ctx)
	if err != nil || got.Name != "n2" || got.ID != "vid" || got.KDF != m.KDF ||
		string(got.Salt) != "\x01\x02" || string(got.WrappedVaultKey) != "\x03\x04" || !got.CreatedAt.Equal(m.CreatedAt) {
		t.Fatalf("meta = %+v %v", got, err)
	}
}

func TestTxRollbackOnError(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	boom := errors.New("boom")
	err := s.Tx(ctx, func(tx app.RecordStore) error {
		if err := tx.Put(ctx, rec("a", "p")); err != nil {
			return err
		}
		if err := tx.Tx(ctx, func(app.RecordStore) error { return nil }); err == nil {
			t.Error("nested Tx must fail")
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Get(ctx, app.KindSecret, "a"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("rollback failed: %v", err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic not propagated")
			}
		}()
		_ = s.Tx(ctx, func(tx app.RecordStore) error {
			_ = tx.Put(ctx, rec("b", "p"))
			panic("x")
		})
	}()
	if _, err := s.Get(ctx, app.KindSecret, "b"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("panic rollback failed: %v", err)
	}
	if err := s.Tx(ctx, func(tx app.RecordStore) error { return tx.Put(ctx, rec("c", "p")) }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, app.KindSecret, "c"); err != nil {
		t.Errorf("commit failed: %v", err)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, path := newStore(t)
	if err := s.Put(ctx, rec("a", "p")); err != nil {
		t.Fatal(err)
	}
	s.(io.Closer).Close()
	for i := 0; i < 2; i++ {
		s2, err := sqlite.Opener{}.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s2.Get(ctx, app.KindSecret, "a"); err != nil {
			t.Fatal(err)
		}
		s2.(io.Closer).Close()
	}
}

func TestOpenMissingIsNotFound(t *testing.T) {
	_, err := sqlite.Opener{}.Open(context.Background(), filepath.Join(t.TempDir(), "x.db"))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestOpenGarbageFile(t *testing.T) {
	for _, n := range []int{4096, 100} {
		path := filepath.Join(t.TempDir(), "g.db")
		b := make([]byte, n)
		rand.Read(b)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := sqlite.Opener{}.Open(context.Background(), path)
		if !errors.Is(err, domain.ErrCorrupt) {
			t.Fatalf("n=%d err = %v", n, err)
		}
		if strings.Contains(err.Error(), string(b[:16])) {
			t.Error("error leaks file content")
		}
	}
}

func TestFutureSchemaVersionRefused(t *testing.T) {
	ctx := context.Background()
	s, path := newStore(t)
	s.(io.Closer).Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES (99)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := (sqlite.Opener{}).Open(ctx, path); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("err = %v", err)
	}
}

func TestTwoConnectionsConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	s1, path := newStore(t)
	s2, err := sqlite.Opener{}.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.(io.Closer).Close()
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i, s := range []app.RecordStore{s1, s2} {
		for j := 0; j < 50; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- s.Put(ctx, rec(fmt.Sprintf("%d-%d", i, j), "p"))
			}()
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if l, err := s1.List(ctx, app.KindSecret, "p"); err != nil || len(l) != 100 {
		t.Errorf("list = %d %v", len(l), err)
	}
}

func TestOpenTinyFilesAreCorruptAndUntouched(t *testing.T) {
	for _, n := range []int{1, 99} {
		path := filepath.Join(t.TempDir(), "t.db")
		b := make([]byte, n)
		rand.Read(b)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := (sqlite.Opener{}).Open(context.Background(), path); !errors.Is(err, domain.ErrCorrupt) {
			t.Fatalf("n=%d err = %v", n, err)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(b) {
			t.Errorf("n=%d file modified", n)
		}
	}
}

func TestOpenTruncatedRealVault(t *testing.T) {
	s, path := newStore(t)
	if err := s.Put(context.Background(), rec("a", "p")); err != nil {
		t.Fatal(err)
	}
	s.(io.Closer).Close()
	if err := os.Truncate(path, 50); err != nil {
		t.Fatal(err)
	}
	os.Remove(path + "-wal")
	os.Remove(path + "-shm")
	before, _ := os.ReadFile(path)
	if _, err := (sqlite.Opener{}).Open(context.Background(), path); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Error("file modified")
	}
}
