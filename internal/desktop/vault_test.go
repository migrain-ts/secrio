package desktop

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"secrio/internal/app"
)

func mustCreate(t *testing.T, env *testEnv, name string) VaultDTO {
	t.Helper()
	v, err := env.app.CreateVault(name, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func noVaultFiles(t *testing.T, dir string) {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "vaults", "*"))
	if len(m) != 0 {
		t.Fatalf("unexpected vault files: %v", m)
	}
}

func TestCreateAndList(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	mustCreate(t, env, "alpha")
	vs, err := env.app.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0].Name != "alpha" || vs[1].Name != "work" {
		t.Fatalf("got %+v", vs)
	}
	if env.newed != 1 {
		t.Fatalf("service created %d times", env.newed)
	}
}

func TestCreateSelectsAndUnlocks(t *testing.T) {
	env := newTestApp(t)
	v := mustCreate(t, env, "work")
	cur, unlocked, err := env.app.CurrentVault()
	if err != nil || !unlocked || cur != v {
		t.Fatalf("cur=%+v unlocked=%v err=%v", cur, unlocked, err)
	}
}

func TestCreateRefusals(t *testing.T) {
	env := newTestApp(t)
	for _, name := range []string{"", ".", "..", "a..b", ".hidden", "-x", "a/b", "a b", "con", "NUL", "com1", "LPT9.txt", strings.Repeat("a", 65)} {
		_, err := env.app.CreateVault(name, testPassword)
		if err == nil || codeOf(t, err) != CodeInvalidName {
			t.Errorf("%q: %v", name, err)
		}
	}
	noVaultFiles(t, env.dir)
	if _, err := env.app.CreateVault("ok", ""); err == nil || codeOf(t, err) != CodeInvalidValue {
		t.Fatalf("empty password: %v", err)
	}
	noVaultFiles(t, env.dir)

	mustCreate(t, env, "Work")
	for _, n := range []string{"Work", "work", "WORK"} {
		if _, err := env.app.CreateVault(n, testPassword); err == nil || codeOf(t, err) != CodeDuplicate {
			t.Errorf("%q: %v", n, err)
		}
	}
	m, _ := filepath.Glob(filepath.Join(env.dir, "vaults", "*.db"))
	if len(m) != 1 {
		t.Fatalf("files: %v", m)
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	env := newTestApp(t)
	v := mustCreate(t, env, "work")
	if err := env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	err := env.app.Unlock(v.ID, "wrong password xyz")
	if err == nil || codeOf(t, err) != CodeInvalidPassword {
		t.Fatalf("got %v", err)
	}
	if _, unlocked, _ := env.app.CurrentVault(); unlocked {
		t.Fatal("must stay locked")
	}
	if err := env.app.Unlock(v.ID, testPassword); err != nil {
		t.Fatal(err)
	}
	if _, unlocked, _ := env.app.CurrentVault(); !unlocked {
		t.Fatal("want unlocked")
	}
	if err := env.app.Unlock("nope", testPassword); err == nil || codeOf(t, err) != CodeNotFound {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestLockThenWithVaultIsLocked(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	called := false
	if err := env.app.withVault(func(context.Context, *app.Service, app.VaultEntry, *app.Unlocked) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
	if err := env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	err := env.app.withVault(func(context.Context, *app.Service, app.VaultEntry, *app.Unlocked) error { return nil })
	if err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("got %v", err)
	}
}

func TestNoVault(t *testing.T) {
	env := newTestApp(t)
	if err := env.app.Lock(); err == nil || codeOf(t, err) != CodeNoVault {
		t.Fatalf("Lock: %v", err)
	}
	err := env.app.withVault(func(context.Context, *app.Service, app.VaultEntry, *app.Unlocked) error { return nil })
	if err == nil || codeOf(t, err) != CodeNoVault {
		t.Fatalf("withVault: %v", err)
	}
}

func TestSessionFromAnotherService(t *testing.T) {
	env := newTestApp(t)
	other, err := env.otherService()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	path := filepath.Join(env.dir, "cli.db")
	v, err := other.CreateVault(ctx, "cli", path, []byte(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Unlock(ctx, string(v.ID), []byte(testPassword), time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	cur, unlocked, err := env.app.CurrentVault() // single vault auto-selects
	if err != nil || !unlocked || cur.Name != "cli" {
		t.Fatalf("cur=%+v unlocked=%v err=%v", cur, unlocked, err)
	}
}

func TestSessionExpiry(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	env.fake.Advance(13 * time.Hour)
	_, unlocked, err := env.app.CurrentVault()
	if err != nil || unlocked {
		t.Fatalf("unlocked=%v err=%v", unlocked, err)
	}
	err = env.app.withVault(func(context.Context, *app.Service, app.VaultEntry, *app.Unlocked) error { return nil })
	if err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("got %v", err)
	}
}

func TestPrefsFile(t *testing.T) {
	env := newTestApp(t)
	v := mustCreate(t, env, "work")
	path := filepath.Join(env.dir, "desktop.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"lastVault":"` + v.ID + `"}`; string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if p := loadPrefs(env.dir); p.LastVault != v.ID {
		t.Fatalf("round trip: %+v", p)
	}
	for name, content := range map[string]string{
		"corrupt":   "{not json",
		"oversized": `{"lastVault":"` + strings.Repeat("x", 5000) + `"}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if p := loadPrefs(env.dir); p.LastVault != "" {
			t.Errorf("%s: got %+v", name, p)
		}
	}
	_ = os.Remove(path)
	if p := loadPrefs(env.dir); p.LastVault != "" {
		t.Fatalf("missing: %+v", p)
	}
	// Only the temp-file-free result remains.
	m, _ := filepath.Glob(filepath.Join(env.dir, "desktop.json.*"))
	if len(m) != 0 {
		t.Fatalf("stray temp files: %v", m)
	}
}

func TestCurrentVaultRules(t *testing.T) {
	env := newTestApp(t)
	// No vaults.
	if v, ok, err := env.app.CurrentVault(); err != nil || ok || v != (VaultDTO{}) {
		t.Fatalf("none: %+v %v %v", v, ok, err)
	}
	a := mustCreate(t, env, "alpha")
	b := mustCreate(t, env, "beta")

	// New App on the same dir: last vault from desktop.json.
	fresh := New(Deps{DataDir: env.app.deps.DataDir, NewService: env.app.deps.NewService})
	cur, _, err := fresh.CurrentVault()
	if err != nil || cur != b {
		t.Fatalf("last: %+v %v", cur, err)
	}
	if err := fresh.SelectVault(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := fresh.SelectVault("missing"); err == nil || codeOf(t, err) != CodeNotFound {
		t.Fatalf("select missing: %v", err)
	}
	if p := loadPrefs(env.dir); p.LastVault != a.ID {
		t.Fatalf("persisted %+v", p)
	}

	// Several vaults and no preference: none.
	if err := os.Remove(filepath.Join(env.dir, "desktop.json")); err != nil {
		t.Fatal(err)
	}
	fresh2 := New(Deps{DataDir: env.app.deps.DataDir, NewService: env.app.deps.NewService})
	if v, ok, err := fresh2.CurrentVault(); err != nil || ok || v != (VaultDTO{}) {
		t.Fatalf("several: %+v %v %v", v, ok, err)
	}
}

func TestShutdown(t *testing.T) {
	newTestApp(t).app.Shutdown() // no vault: safe
	env := newTestApp(t)
	mustCreate(t, env, "work")
	env.app.Shutdown()
	env.app.Shutdown() // idempotent
	if _, unlocked, _ := env.app.CurrentVault(); unlocked {
		t.Fatal("Shutdown must lock")
	}
}

func TestPasswordNeverInErrors(t *testing.T) {
	env := newTestApp(t)
	const pw = "s3cr3t-pw-marker-9f2"
	v, err := env.app.CreateVault("work", pw)
	if err != nil {
		t.Fatal(err)
	}
	_ = env.app.Lock()
	errs := []error{
		env.app.Unlock(v.ID, pw+"x"),
		env.app.Unlock("missing", pw),
	}
	_, e1 := env.app.CreateVault("bad name", pw)
	_, e2 := env.app.CreateVault("WORK", pw)
	_, e3 := env.app.CreateVault(pw+"/..", pw)
	errs = append(errs, e1, e2, e3)
	for _, e := range errs {
		if e == nil {
			t.Fatal("expected failure")
		}
		if strings.Contains(e.Error(), pw) {
			t.Fatalf("password in error: %s", e)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = env.app.ListVaults()
			_, _, _ = env.app.CurrentVault()
		}()
	}
	wg.Wait()
	if env.newed != 1 {
		t.Fatalf("service created %d times", env.newed)
	}
}

func TestNewPanicsOnMissingDeps(t *testing.T) {
	for name, d := range map[string]Deps{
		"service": {DataDir: func() (string, error) { return "", nil }},
		"datadir": {NewService: func() (*app.Service, error) { return nil, nil }},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want panic", name)
				}
			}()
			New(d)
		}()
	}
}

func TestDefaultAfterFunc(t *testing.T) {
	a := New(Deps{DataDir: func() (string, error) { return "", nil }, NewService: func() (*app.Service, error) { return nil, nil }})
	if a.deps.AfterFunc == nil {
		t.Fatal("default AfterFunc missing")
	}
	stop := a.deps.AfterFunc(time.Hour, func() {})
	if !stop() {
		t.Fatal("stop should cancel a pending timer")
	}
}

func TestImportGuard(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.HasPrefix(imp, "secrio/internal/infra") ||
			strings.HasPrefix(imp, "github.com/wailsapp") ||
			strings.HasPrefix(imp, "secrio/internal/cli") {
			t.Errorf("forbidden import %s", imp)
		}
	}
}
