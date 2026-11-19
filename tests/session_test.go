package tests

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"secrio/internal/core"
	"secrio/internal/domain"
	"secrio/internal/infra/clock"
)

const keychainService = "secrio" // core.New's default keychain service

func keychainValue(t *testing.T, e *env) string {
	t.Helper()
	v, err := keyring.Get(keychainService, e.vaultID)
	if err != nil {
		t.Fatalf("keychain item: %v", err)
	}
	return v
}

func assertNoKeychainItem(t *testing.T, e *env) {
	t.Helper()
	if _, err := keyring.Get(keychainService, e.vaultID); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("keychain item still present: %v", err)
	}
}

// sessionBlob returns the sessions row blob (nil if no row). No store may be
// open on the vault.
func sessionBlob(t *testing.T, e *env) []byte {
	t.Helper()
	db := rawDB(t, e.dbPath)
	defer db.Close()
	var b []byte
	err := db.QueryRow(`SELECT blob FROM sessions`).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertLocked(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("got %v, want ErrLocked", err)
	}
}

func TestKeychainHoldsNoVaultKey(t *testing.T) {
	e := newEnv(t) // unlocked once
	first := keychainValue(t, e)
	blob1 := sessionBlob(t, e)

	raw, err := base64.StdEncoding.DecodeString(first)
	if err != nil || len(raw) != 32 {
		t.Fatalf("keychain value is not a base64 32-byte key (len %d, %v)", len(raw), err)
	}
	if len(blob1) <= 32 || string(blob1) == string(raw) {
		t.Fatalf("sessions.blob (len %d) must be a sealed blob distinct from the keychain key", len(blob1))
	}

	e.unlock(t) // a second Unlock draws a fresh session key
	second := keychainValue(t, e)
	if second == first {
		t.Fatal("two Unlocks stored the same keychain value")
	}
	if blob2 := sessionBlob(t, e); string(blob2) == string(blob1) {
		t.Fatal("two Unlocks stored the same session blob")
	}

	// The replaced (old) session key opens nothing.
	if err := keyring.Delete(keychainService, e.vaultID); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set(keychainService, e.vaultID, first); err != nil { // the old, replaced key
		t.Fatal(err)
	}
	_, err = e.svc.Open(context.Background(), e.vaultID)
	assertLocked(t, err)
}

func TestSessionSharedAndKilledAcrossServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	svcB, err := core.New(e.dir, core.WithKDFParams(cheapKDF))
	if err != nil {
		t.Fatal(err)
	}
	ub, err := svcB.Open(ctx, e.vaultID)
	if err != nil {
		t.Fatalf("B with A's session: %v", err)
	}
	ub.Close()

	if err := e.svc.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	_, err = svcB.Open(ctx, e.vaultID)
	assertLocked(t, err)
	assertNoKeychainItem(t, e)
	if sessionBlob(t, e) != nil {
		t.Error("session row survived Lock")
	}
}

func TestLeakedKeychainItemUselessAfterLock(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	leaked := keychainValue(t, e)

	if err := e.svc.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	// Attacker puts the leaked key back: there is no row to open.
	if err := keyring.Set(keychainService, e.vaultID, leaked); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Open(ctx, e.vaultID)
	assertLocked(t, err)
	assertNoKeychainItem(t, e)

	// A fresh Unlock makes a new session; the old key does not open it.
	e.unlock(t)
	fresh := keychainValue(t, e)
	if fresh == leaked {
		t.Fatal("session key reused across Unlocks")
	}
	if err := keyring.Set(keychainService, e.vaultID, leaked); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Open(ctx, e.vaultID)
	assertLocked(t, err)
	assertNoKeychainItem(t, e)
	if sessionBlob(t, e) != nil {
		t.Error("session row survived the failed Open")
	}
}

func TestRowWithoutKeychainKeyIsLockedAndCleaned(t *testing.T) {
	e := newEnv(t)
	if err := keyring.Delete(keychainService, e.vaultID); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Open(context.Background(), e.vaultID)
	assertLocked(t, err)
	if sessionBlob(t, e) != nil {
		t.Error("orphan session row not deleted")
	}
}

func TestKeychainKeyWithoutRowIsLocked(t *testing.T) {
	e := newEnv(t)
	db := rawDB(t, e.dbPath)
	if _, err := db.Exec(`DELETE FROM sessions`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err := e.svc.Open(context.Background(), e.vaultID)
	assertLocked(t, err)
	assertNoKeychainItem(t, e)
}

func TestSessionExpiryEndToEnd(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	t.Run("ttl", func(t *testing.T) {
		clk := clock.NewFake(start)
		e := newEnv(t, core.WithClock(clk))
		clk.Advance(59 * time.Minute)
		u, err := e.svc.Open(ctx, e.vaultID)
		if err != nil {
			t.Fatalf("before ttl: %v", err)
		}
		u.Close()
		clk.Advance(time.Minute)
		_, err = e.svc.Open(ctx, e.vaultID)
		assertLocked(t, err)
		assertNoKeychainItem(t, e)
		if sessionBlob(t, e) != nil {
			t.Error("expired session row not purged")
		}
	})

	t.Run("idle", func(t *testing.T) {
		clk := clock.NewFake(start)
		e := newEnv(t, core.WithClock(clk))
		if err := e.svc.Unlock(ctx, e.vaultID, []byte(testPw), 8*hour, 10*time.Minute); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ { // each use refreshes the idle timer
			clk.Advance(9 * time.Minute)
			u, err := e.svc.Open(ctx, e.vaultID)
			if err != nil {
				t.Fatalf("use %d: %v", i, err)
			}
			u.Close()
		}
		clk.Advance(10 * time.Minute)
		_, err := e.svc.Open(ctx, e.vaultID)
		assertLocked(t, err)
		assertNoKeychainItem(t, e)
		if sessionBlob(t, e) != nil {
			t.Error("idle session row not purged")
		}
	})
}
