package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
	"secrio/internal/infra/keychain"
	"secrio/internal/infra/registry"
)

func TestE2ECreateUnlockSetCloseReopenGet(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	u := e.open(t)
	if _, err := u.SetSecret(ctx, e.dev.ID, "API_KEY", []byte("v-12345"), domain.TypeAPIKey, "the key"); err != nil {
		t.Fatal(err)
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}

	// A brand-new service on the same dir sees the vault and the session.
	svc2, err := core.New(e.dir, core.WithKDFParams(cheapKDF))
	if err != nil {
		t.Fatal(err)
	}
	vaults, err := svc2.ListVaults(ctx)
	if err != nil || len(vaults) != 1 || vaults[0].Name != "My Vault" {
		t.Fatalf("vaults = %+v, %v", vaults, err)
	}
	u2, err := svc2.Open(ctx, e.vaultID)
	if err != nil {
		t.Fatal(err)
	}
	defer u2.Close()
	got, err := u2.GetSecret(ctx, e.dev.ID, "API_KEY")
	if err != nil || string(got.Value) != "v-12345" || got.Type != domain.TypeAPIKey || got.Description != "the key" {
		t.Fatalf("secret = %v, %v", got.Name, err)
	}
	if err := svc2.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Open(ctx, e.vaultID); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("after lock: %v", err)
	}
}

func TestSessionSharedBetweenTwoServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t) // service A; unlocked
	svcB, err := core.New(e.dir, core.WithKDFParams(cheapKDF))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	if _, err := svcB.Open(ctx, e.vaultID); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("B after A.Lock: %v", err)
	}
	e.unlock(t)
	ub, err := svcB.Open(ctx, e.vaultID)
	if err != nil {
		t.Fatalf("B after A.Unlock: %v", err)
	}
	ub.Close()
	if err := e.svc.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	if _, err := svcB.Open(ctx, e.vaultID); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("B after second Lock: %v", err)
	}
}

func TestConcurrentServicesRace(t *testing.T) {
	ctx := context.Background()
	// The go-keyring mock is not goroutine-safe, so share a memory store.
	keys := keychain.NewMemory()
	e := newEnv(t, core.WithSessionKeys(keys))
	svcB, err := core.New(e.dir, core.WithKDFParams(cheapKDF), core.WithSessionKeys(keys))
	if err != nil {
		t.Fatal(err)
	}
	const n = 15
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	run := func(open func() (*app.Unlocked, error), envID domain.ID[domain.Environment], prefix string) {
		defer wg.Done()
		u, err := open()
		if err != nil {
			errs <- err
			return
		}
		defer u.Close()
		for i := 0; i < n; i++ {
			name := domain.SecretName(prefix + string(rune('A'+i)))
			if _, err := u.SetSecret(ctx, envID, name, []byte("v"), "", ""); err != nil {
				errs <- err
				return
			}
		}
	}
	wg.Add(2)
	go run(func() (*app.Unlocked, error) { return e.svc.Open(ctx, e.vaultID) }, e.dev.ID, "DEV_")
	go run(func() (*app.Unlocked, error) { return svcB.Open(ctx, e.vaultID) }, e.prd.ID, "PRD_")
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	u := e.open(t)
	defer u.Close()
	for _, id := range []domain.ID[domain.Environment]{e.dev.ID, e.prd.ID} {
		ss, err := u.ListSecrets(ctx, id)
		if err != nil || len(ss) != n {
			t.Fatalf("env %s: %d secrets, %v", id, len(ss), err)
		}
	}
}

func TestRunEnvEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	ctx := context.Background()
	e := newEnv(t)
	u := e.open(t)
	defer u.Close()
	if _, err := u.SetSecret(ctx, e.dev.ID, "MY_SECRET", []byte("child-sees-this"), "", ""); err != nil {
		t.Fatal(err)
	}
	environ, err := u.ProcessEnv(ctx, e.dev.ID, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `printf %s "$MY_SECRET"; exit 3`)
	cmd.Env = environ
	out, err := cmd.Output()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("err = %v", err)
	}
	if string(out) != "child-sees-this" {
		t.Fatalf("child output = %q", out)
	}
}

func TestVaultPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "a/b", `a\b`, "..", "../x", "a..b", "a\x00b", "/abs"} {
		if p, err := core.VaultPath(dir, bad); err == nil {
			t.Errorf("VaultPath(%q) = %q, want error", bad, p)
		}
	}
	p, err := core.VaultPath(dir, "main")
	if err != nil || p != filepath.Join(dir, "vaults", "main.db") {
		t.Fatalf("VaultPath = %q, %v", p, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "vaults")); err != nil || !fi.IsDir() {
		t.Fatalf("vaults dir: %v", err)
	}
}

func TestDefaultDataDir(t *testing.T) {
	d, err := core.DefaultDataDir()
	if err != nil || filepath.Base(d) != "secrio" || !filepath.IsAbs(d) {
		t.Fatalf("DefaultDataDir = %q, %v", d, err)
	}
}

func TestNewCreatesDataDir(t *testing.T) {
	keyring.MockInit()
	dir := filepath.Join(t.TempDir(), "nested", "secrio")
	if _, err := core.New(dir); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("data dir: %v", err)
	}
}

func TestWrongPasswordAndCorruptedFile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if err := e.svc.Unlock(ctx, e.vaultID, []byte("wrong"), hour, 0); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("wrong password: %v", err)
	}

	const marker = "GARBAGE-MARKER-XYZ"
	garbage := filepath.Join(e.dir, "vaults", "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat(marker, 400)), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := registry.New(e.dir)
	if err := reg.Add(ctx, app.VaultEntry{ID: "garbage-id", Name: "garbage", Path: garbage}); err != nil {
		t.Fatal(err)
	}
	err := e.svc.Unlock(ctx, "garbage-id", []byte(testPw), hour, 0)
	if !errors.Is(err, domain.ErrCorrupt) || strings.Contains(err.Error(), marker) {
		t.Fatalf("Unlock garbage: %v", err)
	}
	_, err = e.svc.OpenWithPassword(ctx, "garbage-id", []byte(testPw))
	if !errors.Is(err, domain.ErrCorrupt) || strings.Contains(err.Error(), marker) {
		t.Fatalf("OpenWithPassword garbage: %v", err)
	}
	// Open reads the vault file before the keychain: the file defect wins.
	_, err = e.svc.Open(ctx, "garbage-id")
	if !errors.Is(err, domain.ErrCorrupt) || strings.Contains(err.Error(), marker) {
		t.Fatalf("Open garbage without session: %v", err)
	}
}
