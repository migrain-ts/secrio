package tests

import (
	"context"
	"testing"
	"time"
)

// A file-only writer who edits expires_at or idle_ms breaks the blob's AAD:
// Open fails closed and both session halves are removed.
func TestTamperedSessionTimingColumnsAreLocked(t *testing.T) {
	ctx := context.Background()
	for name, stmt := range map[string]string{
		"expires_at": `UPDATE sessions SET expires_at = expires_at + 86400000`,
		"idle_ms":    `UPDATE sessions SET idle_ms = 0`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if err := e.svc.Unlock(ctx, e.vaultID, []byte(testPw), hour, 10*time.Minute); err != nil {
				t.Fatal(err)
			}
			db := rawDB(t, e.dbPath)
			res, err := db.Exec(stmt)
			if err != nil {
				t.Fatal(err)
			}
			if n, _ := res.RowsAffected(); n != 1 {
				t.Fatalf("tamper touched %d rows", n)
			}
			db.Close()
			_, err = e.svc.Open(ctx, e.vaultID)
			assertLocked(t, err)
			assertNoKeychainItem(t, e)
			if sessionBlob(t, e) != nil {
				t.Error("session row survived")
			}
		})
	}
}

func TestUntamperedSessionWithIdleOpens(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if err := e.svc.Unlock(ctx, e.vaultID, []byte(testPw), hour, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	e.open(t).Close()
}
