package app

import (
	"context"
	"testing"
	"time"
)

func TestTamperedSessionTimingIsLocked(t *testing.T) {
	for name, tamper := range map[string]func(*SessionRow){
		"expires_at": func(r *SessionRow) { r.ExpiresAt = r.ExpiresAt.Add(24 * time.Hour) },
		"idle":       func(r *SessionRow) { r.Idle = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			id := h.newVault(t)
			h.unlock(t, id, time.Hour, 10*time.Minute)
			tamper(h.vault().session)
			_, err := h.svc.Open(context.Background(), id)
			assertLocked(t, err)
			h.assertNoSession(t, id)
		})
	}
}

func TestUntamperedSessionTimingOpens(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 10*time.Minute)
	u, err := h.svc.Open(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	u.Close()
}

func TestSessionAADBindsVaultAndTiming(t *testing.T) {
	exp := time.UnixMilli(1700000000000)
	base := string(sessionAAD("v1", exp, time.Minute))
	if want := "session|v1|1700000000000|60000"; base != want {
		t.Fatalf("aad = %q, want %q", base, want)
	}
	for name, other := range map[string]string{
		"vault":   string(sessionAAD("v2", exp, time.Minute)),
		"expiry":  string(sessionAAD("v1", exp.Add(time.Millisecond), time.Minute)),
		"idle":    string(sessionAAD("v1", exp, 2*time.Minute)),
		"no idle": string(sessionAAD("v1", exp, 0)),
	} {
		if other == base {
			t.Errorf("aad does not change with %s", name)
		}
	}
	// A sub-millisecond idle is stored as 1 ms, and the AAD agrees.
	if string(sessionAAD("v1", exp, time.Microsecond)) != string(sessionAAD("v1", exp, time.Millisecond)) {
		t.Error("sub-ms idle must normalise to 1 ms")
	}
}
