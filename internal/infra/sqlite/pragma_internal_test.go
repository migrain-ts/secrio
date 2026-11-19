package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenedStoreHasSecureDelete(t *testing.T) {
	st, err := open(context.Background(), filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var v int
	if err := st.db.QueryRow("PRAGMA secure_delete").Scan(&v); err != nil || v != 1 {
		t.Fatalf("secure_delete = %d, %v", v, err)
	}
}

func TestBusyTimeoutPragmaBeforeWAL(t *testing.T) {
	d := dsn("/x/v.db")
	bt, wal := strings.Index(d, "busy_timeout"), strings.Index(d, "journal_mode")
	if bt < 0 || wal < 0 || bt > wal {
		t.Fatalf("dsn pragma order wrong: %s", d)
	}
}
