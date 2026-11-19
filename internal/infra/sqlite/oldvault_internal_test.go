package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"secrio/internal/app"
)

// A vault file written before the sessions table existed gains it when
// opened through the Opener, and sessions then work on it.
func TestOldVaultFileGainsSessionsTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateWith(ctx, db, ms[:1]); err != nil { // schema as of migration 0001 only
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT 1 FROM sessions`); err == nil {
		t.Fatal("test setup: sessions table already exists")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Opener{}.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.UnixMilli(1700000000000)
	if _, err := st.GetSession(ctx); err == nil {
		t.Fatal("old vault reports a session")
	}
	if err := st.PutSession(ctx, app.SessionRow{Blob: []byte("b"), ExpiresAt: now.Add(time.Hour), LastUsed: now}); err != nil {
		t.Fatalf("PutSession on migrated file: %v", err)
	}
	if ok, err := st.TouchSession(ctx, now); err != nil || !ok {
		t.Fatalf("TouchSession = %v, %v", ok, err)
	}
}
