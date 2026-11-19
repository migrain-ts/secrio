package cli

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
	"secrio/internal/infra/keychain"
)

// flakyKeys is a SessionKeys whose Load can be made to fail.
type flakyKeys struct {
	inner   app.SessionKeys
	loadErr error
	loads   int
}

func (f *flakyKeys) Save(ctx context.Context, id string, k domain.Key) error {
	return f.inner.Save(ctx, id, k)
}

func (f *flakyKeys) Load(ctx context.Context, id string) (domain.Key, error) {
	f.loads++
	if f.loadErr != nil {
		return domain.Key{}, f.loadErr
	}
	return f.inner.Load(ctx, id)
}

func (f *flakyKeys) Delete(ctx context.Context, id string) error { return f.inner.Delete(ctx, id) }

const keychainDown = "keychain unavailable"

// flakyEnv is secretsEnv with vault "work" holding K=value, locked, running on
// a Core whose session keystore can fail.
func flakyEnv(t *testing.T) (*env, *flakyKeys) {
	t.Helper()
	e := secretsEnv(t)
	fk := &flakyKeys{inner: keychain.NewMemory()}
	svc, err := core.New(e.dir, core.WithKDFParams(cheapKDF), core.WithSessionKeys(fk))
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	e.d.NewService = func() (*app.Service, error) { return svc, nil }
	e.d.Run = systemRunner(nil, nil)
	e.d.Environ = func() []string { return nil }
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.secret([]byte("value"), "set", "K"); code != ExitOK {
		t.Fatalf("setup set: %d %q", code, e.err)
	}
	delete(e.vars, "SECRIO_PASSWORD")
	fk.loadErr = errors.New(keychainDown)
	return e, fk
}

func TestKeychainDownPasswordFallback(t *testing.T) {
	e, fk := flakyEnv(t)
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "value" {
		t.Fatalf("get: code=%d out=%q err=%q", code, e.out, e.err)
	}
	if code := e.secret(nil, "list"); code != ExitOK || e.out.String() != "K\n" {
		t.Fatalf("list: code=%d out=%q err=%q", code, e.out, e.err)
	}
	if runtime.GOOS != "windows" {
		if code := e.secret(nil, "run", "--", "sh", "-c", `printf %s "$K"`); code != ExitOK || e.out.String() != "value" {
			t.Fatalf("run: code=%d out=%q err=%q", code, e.out, e.err)
		}
	}
	// No session was created.
	fk.loadErr = nil
	u, err := e.svc.Open(context.Background(), e.lookup("work").ID)
	if err == nil {
		u.Close()
	}
	if !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("expected no session, Open err=%v", err)
	}
}

func TestKeychainDownWithoutPassword(t *testing.T) {
	e, _ := flakyEnv(t)
	code := e.secret(nil, "get", "K")
	if code != ExitFailure || !strings.Contains(e.err.String(), keychainDown) || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if !strings.HasSuffix(e.err.String(), "(set SECRIO_PASSWORD to open without the OS keychain)\n") {
		t.Fatalf("no hint: %q", e.err)
	}
}

func TestKeychainDownWrongPassword(t *testing.T) {
	e, _ := flakyEnv(t)
	e.vars["SECRIO_PASSWORD"] = "wrong " + testPassword
	if code := e.secret(nil, "get", "K"); code != ExitLocked || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if strings.Contains(e.err.String(), testPassword) {
		t.Fatalf("password in error: %q", e.err)
	}
}

func TestLiveSessionBeatsPassword(t *testing.T) {
	e, fk := flakyEnv(t)
	fk.loadErr = nil
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.run(nil, "unlock", "work"); code != ExitOK {
		t.Fatalf("unlock: %d %q", code, e.err)
	}
	e.vars["SECRIO_PASSWORD"] = "wrong " + testPassword
	before := fk.loads
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "value" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if fk.loads == before {
		t.Fatal("session was not consulted")
	}
}
