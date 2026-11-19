package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

func testMigrations(t *testing.T) []migration {
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	return append(ms, migration{version: 2, sql: `CREATE TABLE extra (x INTEGER);`})
}

func countVersions(t *testing.T, db *sql.DB) map[int]int {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[int]int{}
	for rows.Next() {
		var v int
		rows.Scan(&v)
		got[v]++
	}
	return got
}

func TestApplyMigrationSkipsAlreadyApplied(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v.db")
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ms := testMigrations(t)
	if err := migrateWith(ctx, db, ms); err != nil {
		t.Fatal(err)
	}
	// Simulate a stale reader: apply 0002 again after it is already recorded.
	if err := applyMigration(ctx, db, ms[1]); err != nil {
		t.Fatalf("stale re-apply: %v", err)
	}
	if c := countVersions(t, db); c[1] != 1 || c[2] != 1 {
		t.Errorf("versions = %v", c)
	}
}

func TestConcurrentMigrationFromV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v.db")
	ms := testMigrations(t)
	seed, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Close()
	if err := migrateWith(ctx, seed, ms[:1]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		db, err := sql.Open("sqlite", dsn(path))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		wg.Add(1)
		go func() { defer wg.Done(); errs <- migrateWith(ctx, db, ms) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if c := countVersions(t, seed); c[1] != 1 || c[2] != 1 {
		t.Errorf("versions = %v", c)
	}
}
