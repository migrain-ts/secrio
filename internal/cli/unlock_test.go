package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"secrio/internal/domain"
)

func mkVault(t *testing.T, e *env, name string) {
	t.Helper()
	createVault(t, e.svc, e.dir, name)
}

func isLocked(t *testing.T, e *env, name string) bool {
	t.Helper()
	u, err := e.svc.Open(context.Background(), e.lookup(name).ID)
	if err == nil {
		_ = u.Close()
		return false
	}
	if !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("open: %v", err)
	}
	return true
}

func TestUnlockThenOpenVaultThenLock(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	if code := e.run([]string{testPassword}, "unlock", "work"); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if e.out.String() != "unlocked work (expires in 12h0m0s)\n" {
		t.Fatalf("out=%q", e.out)
	}
	if e.prompts[0] != "Password: " {
		t.Fatalf("prompt=%q", e.prompts[0])
	}
	entry := e.lookup("work")
	u, err := openVault(context.Background(), e.d, e.svc, entry)
	if err != nil {
		t.Fatal(err)
	}
	_ = u.Close()

	if code := e.run(nil, "lock", "work"); code != ExitOK || e.out.String() != "locked work\n" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	_, err = openVault(context.Background(), e.d, e.svc, entry)
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitLocked || !strings.Contains(err.Error(), "secrio unlock") {
		t.Fatalf("err=%v", err)
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	if code := e.run([]string{"wrong"}, "unlock", "work"); code != ExitLocked {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if !isLocked(t, e, "work") {
		t.Fatal("vault not locked")
	}
	if _, err := openVault(context.Background(), e.d, e.svc, e.lookup("work")); err == nil {
		t.Fatal("openVault succeeded")
	}
}

func TestUnlockFlags(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	for _, a := range [][]string{{"--ttl", "0"}, {"--ttl", "-1h"}, {"--idle", "-1s"}} {
		args := append([]string{"unlock", "work"}, a...)
		if code := e.run([]string{testPassword}, args...); code != ExitUsage {
			t.Errorf("%v: code=%d", a, code)
		}
		if e.pwCalls != 0 {
			t.Errorf("%v: password prompted", a)
		}
	}
	if code := e.run([]string{testPassword}, "unlock", "work", "--ttl", "2h", "--idle", "0"); code != ExitOK || e.out.String() != "unlocked work (expires in 2h0m0s)\n" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestUnlockUnknownVault(t *testing.T) {
	e := newEnv(t)
	if code := e.run([]string{testPassword}, "unlock", "nope"); code != ExitFailure {
		t.Fatalf("code=%d", code)
	}
	if code := e.run(nil, "lock", "nope"); code != ExitFailure {
		t.Fatalf("lock code=%d", code)
	}
}

func TestUnlockNoVaultIsUsage(t *testing.T) {
	e := newEnv(t)
	if code := e.run(nil, "unlock"); code != ExitUsage {
		t.Fatalf("code=%d", code)
	}
	if code := e.run(nil, "lock"); code != ExitUsage {
		t.Fatalf("lock code=%d", code)
	}
}

func TestUnlockVaultFromContextAndArgumentWins(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "a")
	mkVault(t, e, "b")
	if code := e.run([]string{testPassword}, "--vault", "a", "unlock"); code != ExitOK || e.out.String() != "unlocked a (expires in 12h0m0s)\n" {
		t.Fatalf("flag: code=%d out=%q err=%q", code, e.out, e.err)
	}
	e.run(nil, "lock", "a")
	e.vars["SECRIO_VAULT"] = "a"
	if code := e.run([]string{testPassword}, "unlock", "b"); code != ExitOK || !strings.HasPrefix(e.out.String(), "unlocked b ") {
		t.Fatalf("arg: code=%d out=%q err=%q", code, e.out, e.err)
	}
	if !isLocked(t, e, "a") || isLocked(t, e, "b") {
		t.Fatal("wrong vault unlocked")
	}
	if code := e.run(nil, "lock"); code != ExitOK || e.out.String() != "locked a\n" {
		t.Fatalf("env lock: code=%d out=%q", code, e.out)
	}
}

func TestUnlockNoTTYNoEnv(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	e.tty = false
	if code := e.run([]string{testPassword}, "unlock", "work"); code != ExitLocked || e.pwCalls != 0 {
		t.Fatalf("code=%d calls=%d", code, e.pwCalls)
	}
	if !isLocked(t, e, "work") {
		t.Fatal("unlocked")
	}
}

func TestUnlockEnvPassword(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	e.vars["SECRIO_PASSWORD"] = testPassword
	e.tty = false
	if code := e.run(nil, "unlock", "work"); code != ExitOK || e.pwCalls != 0 {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestOpenVaultWithEnvPasswordNoSession(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	e.vars["SECRIO_PASSWORD"] = testPassword
	entry := e.lookup("work")
	u, err := openVault(context.Background(), e.d, e.svc, entry)
	if err != nil {
		t.Fatal(err)
	}
	_ = u.Close()
	if !isLocked(t, e, "work") {
		t.Fatal("session was created")
	}
	e.vars["SECRIO_PASSWORD"] = "wrong"
	if _, err := openVault(context.Background(), e.d, e.svc, entry); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("err=%v", err)
	}
}

func TestInvalidPasswordFromEnvSaysSo(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	const want = "secrio: invalid password (from SECRIO_PASSWORD)\n"
	e.vars["SECRIO_PASSWORD"] = "wrong-env-pw"
	if code := e.run(nil, "unlock", "work"); code != ExitLocked || e.err.String() != want {
		t.Fatalf("unlock: code=%d err=%q", code, e.err)
	}
	e.err.Reset()
	if code := e.run(nil, "list", "--vault", "work"); code != ExitLocked || e.err.String() != want {
		t.Fatalf("list: code=%d err=%q", code, e.err)
	}
	delete(e.vars, "SECRIO_PASSWORD")
	e.err.Reset()
	if code := e.run([]string{"wrong"}, "unlock", "work"); code != ExitLocked || e.err.String() != "secrio: invalid password\n" {
		t.Fatalf("prompted: code=%d err=%q", code, e.err)
	}
}

func TestLockFailureMessage(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	entry := e.lookup("work")
	if err := os.Remove(entry.Path); err != nil {
		t.Fatal(err)
	}
	code := e.run(nil, "lock", "work")
	if code != ExitFailure || !strings.Contains(e.err.String(), "may still be unlocked") || strings.Contains(e.out.String(), "locked") {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestUnlockWrongPasswordMessageIsGeneric(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	const pw = "wrong-pw-MARKER-91"
	if code := e.run([]string{pw}, "unlock", "work"); code != ExitLocked || e.err.String() != "secrio: invalid password\n" || e.out.Len() != 0 {
		t.Fatalf("prompt: code=%d out=%q err=%q", code, e.out, e.err)
	}
	e.vars["SECRIO_PASSWORD"] = pw
	if code := e.run(nil, "unlock", "work"); code != ExitLocked || e.err.String() != "secrio: invalid password (from SECRIO_PASSWORD)\n" || e.out.Len() != 0 {
		t.Fatalf("env: code=%d out=%q err=%q", code, e.out, e.err)
	}
	if strings.Contains(e.out.String()+e.err.String(), pw) {
		t.Fatal("password printed")
	}
}

func TestUnlockAlreadyUnlockedReplacesSession(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	if code := e.run([]string{testPassword}, "unlock", "work", "--ttl", "1h", "--idle", "0"); code != ExitOK {
		t.Fatalf("first: code=%d err=%q", code, e.err)
	}
	if code := e.run([]string{testPassword}, "unlock", "work", "--ttl", "5h", "--idle", "0"); code != ExitOK || e.out.String() != "unlocked work (expires in 5h0m0s)\n" {
		t.Fatalf("second: code=%d out=%q err=%q", code, e.out, e.err)
	}
	e.clock.Advance(2 * time.Hour) // past the first session's TTL, inside the second's
	if code := e.run(nil, "--vault", "work", "project", "create", "p"); code != ExitOK {
		t.Fatalf("session not replaced: code=%d err=%q", code, e.err)
	}
	// A wrong password on an already unlocked vault fails and leaves the session alone.
	if code := e.run([]string{"wrong"}, "unlock", "work"); code != ExitLocked {
		t.Fatalf("wrong: code=%d", code)
	}
	if code := e.run(nil, "--vault", "work", "list"); code != ExitOK || e.out.String() != "p\n" {
		t.Fatalf("after wrong: code=%d out=%q err=%q", code, e.out, e.err)
	}
}
