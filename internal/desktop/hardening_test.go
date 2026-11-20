package desktop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"secrio/internal/domain"
)

// lockedViaOther reports whether another Service (as the CLI would) sees
// vaultID as locked.
func lockedViaOther(t *testing.T, env *testEnv, vaultID string) bool {
	t.Helper()
	other, err := env.otherService()
	if err != nil {
		t.Fatal(err)
	}
	u, err := other.Open(context.Background(), vaultID)
	if err == nil {
		_ = u.Close()
		return false
	}
	if !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("open: %v", err)
	}
	return true
}

// cliVault creates and unlocks a vault through another Service.
func cliVault(t *testing.T, env *testEnv, name string) string {
	t.Helper()
	other, err := env.otherService()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	v, err := other.CreateVault(ctx, name, filepath.Join(env.dir, name+".db"), []byte(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Unlock(ctx, string(v.ID), []byte(testPassword), time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	return string(v.ID)
}

func TestLockNoVaultStillClears(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	cliVault(t, c.testEnv, "other") // two vaults, no selection, no prefs
	c.app.setCurrent("")
	if err := os.Remove(filepath.Join(c.dir, prefsFile)); err != nil {
		t.Fatal(err)
	}
	if err := c.app.Lock(); err == nil || codeOf(t, err) != CodeNoVault {
		t.Fatalf("got %v", err)
	}
	if c.clip.current() != "" {
		t.Fatal("Lock must clear on the no_vault path")
	}
}

func TestLockPanicStillClears(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.keys.panicDelete.Store(true)
	if err := c.app.Lock(); err == nil || codeOf(t, err) != CodeInternal {
		t.Fatalf("a panicking lock must be reported, got %v", err)
	}
	if c.clip.current() != "" {
		t.Fatal("Lock must clear even when the lock path panics")
	}
}

func TestShutdownPanicStillClears(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.keys.panicDelete.Store(true)
	c.app.Shutdown() // must not panic
	if c.clip.current() != "" {
		t.Fatal("Shutdown must clear even when the lock path panics")
	}
}

func TestShutdownTwiceClearsOnce(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.app.Shutdown()
	c.app.Shutdown()
	if c.clip.clears() != 1 {
		t.Fatalf("clears=%d", c.clip.clears())
	}
}

func TestShutdownDoesNotWritePrefs(t *testing.T) {
	t.Run("no file", func(t *testing.T) {
		env := newTestApp(t)
		cliVault(t, env, "cli") // the only vault: resolveCurrent would persist it
		env.app.Shutdown()
		if _, err := os.Stat(filepath.Join(env.dir, prefsFile)); !os.IsNotExist(err) {
			t.Fatalf("desktop.json created: %v", err)
		}
	})
	t.Run("existing file", func(t *testing.T) {
		env := newTestApp(t)
		a := mustCreate(t, env, "alpha")
		mustCreate(t, env, "beta")
		path := filepath.Join(env.dir, prefsFile)
		if err := savePrefs(env.dir, prefs{LastVault: a.ID}); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)
		fresh := New(Deps{DataDir: env.app.deps.DataDir, NewService: env.app.deps.NewService})
		fresh.Shutdown() // read-only lookup of lastVault
		after, _ := os.ReadFile(path)
		fi, err := os.Stat(path)
		if err != nil || !bytes.Equal(before, after) || !fi.ModTime().Equal(old) {
			t.Fatalf("desktop.json modified: %s -> %s (%v)", before, after, err)
		}
		if !lockedViaOther(t, env, a.ID) {
			t.Fatal("remembered vault must be locked")
		}
	})
}

func TestShutdownLocksAllUnlockedVaults(t *testing.T) {
	env := newTestApp(t)
	a := mustCreate(t, env, "alpha")
	b := mustCreate(t, env, "beta")
	env.app.Shutdown()
	for _, id := range []string{a.ID, b.ID} {
		if !lockedViaOther(t, env, id) {
			t.Fatalf("%s still unlocked", id)
		}
	}
}

func TestLockLocksAllUnlockedVaults(t *testing.T) {
	env := newTestApp(t)
	a := mustCreate(t, env, "alpha")
	b := mustCreate(t, env, "beta") // current
	if err := env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if !lockedViaOther(t, env, id) {
			t.Fatalf("%s still unlocked", id)
		}
	}
}

// After Lock the App no longer owns the sessions: one the CLI opens later
// must survive Shutdown.
func TestShutdownAfterLockKeepsExternalSession(t *testing.T) {
	env := newTestApp(t)
	a := mustCreate(t, env, "alpha")
	b := mustCreate(t, env, "beta") // current
	if err := env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	other, err := env.otherService()
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Unlock(context.Background(), a.ID, []byte(testPassword), time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	env.app.Shutdown()
	if lockedViaOther(t, env, a.ID) {
		t.Fatal("Shutdown ended a session the App did not open")
	}
	if !lockedViaOther(t, env, b.ID) {
		t.Fatal("current vault must stay locked")
	}
}

func TestLockFailureKeepsVaultTracked(t *testing.T) {
	env := newTestApp(t)
	a := mustCreate(t, env, "alpha")
	b := mustCreate(t, env, "beta") // current
	env.keys.failDelete = a.ID
	if err := env.app.Lock(); err == nil || codeOf(t, err) != CodeInternal {
		t.Fatalf("got %v", err)
	}
	env.app.mu.Lock()
	trackedA, trackedB := env.app.unlocked[a.ID], env.app.unlocked[b.ID]
	env.app.mu.Unlock()
	if !trackedA || trackedB {
		t.Fatalf("tracked alpha=%v beta=%v", trackedA, trackedB)
	}
	if !lockedViaOther(t, env, b.ID) {
		t.Fatal("the other vault must still be locked")
	}
}

func TestUnlockAfterShutdownEndsLocked(t *testing.T) {
	env := newTestApp(t)
	v := mustCreate(t, env, "work")
	env.app.Shutdown()
	err := env.app.Unlock(v.ID, testPassword)
	if err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("got %v", err)
	}
	if !lockedViaOther(t, env, v.ID) {
		t.Fatal("late unlock must be locked again")
	}
	if _, err := env.app.CreateVault("late", testPassword); err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("create after shutdown: %v", err)
	}
}

// Shutdown runs while an Unlock is between its password check and its end
// (the late-Argon2 race).
func TestUnlockFinishingDuringShutdown(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	id := cliVault(t, env, "cli")
	other, err := env.otherService()
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Lock(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	env.keys.onSave = func() {
		env.keys.onSave = nil
		env.app.Shutdown()
	}
	err = env.app.Unlock(id, testPassword)
	if err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("got %v", err)
	}
	if !lockedViaOther(t, env, id) {
		t.Fatal("vault unlocked after Shutdown")
	}
}

func TestPrefsWriteFailureNonFatal(t *testing.T) {
	env := newTestApp(t)
	// A directory where desktop.json should be: every write fails.
	if err := os.Mkdir(filepath.Join(env.dir, prefsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	a, err := env.app.CreateVault("alpha", testPassword)
	if err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	b, err := env.app.CreateVault("beta", testPassword)
	if err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	if cur, ok, err := env.app.CurrentVault(); err != nil || !ok || cur != b {
		t.Fatalf("cur=%+v ok=%v err=%v", cur, ok, err)
	}
	if err := env.app.SelectVault(a.ID); err != nil {
		t.Fatalf("SelectVault: %v", err)
	}
	if env.app.getCurrent() != a.ID {
		t.Fatal("in-memory selection lost")
	}
	if err := env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := env.app.Unlock(a.ID, testPassword); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestCreateVaultSelectedEvenIfUnlockFails(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "alpha")
	env.keys.failSave.Store(true)
	if _, err := env.app.CreateVault("beta", testPassword); err == nil {
		t.Fatal("want unlock failure")
	}
	env.keys.failSave.Store(false)
	cur, unlocked, err := env.app.CurrentVault()
	if err != nil || cur.Name != "beta" || unlocked {
		t.Fatalf("cur=%+v unlocked=%v err=%v", cur, unlocked, err)
	}
	if err := env.app.Unlock(cur.ID, testPassword); err != nil {
		t.Fatalf("retry Unlock: %v", err)
	}
}
