package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"secrio/internal/app"
)

// env is a scripted CLI environment over a real Core.
type env struct {
	t        *testing.T
	svc      *app.Service
	dir      string
	d        Deps
	out, err *bytes.Buffer
	vars     map[string]string
	tty      bool
	prompts  []string
	pwQueue  []string
	pwCalls  int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	svc, dir, _ := newTestCore(t)
	e := &env{t: t, svc: svc, dir: dir, out: &bytes.Buffer{}, err: &bytes.Buffer{}, vars: map[string]string{}, tty: true}
	e.d = Deps{
		In:       strings.NewReader(""),
		Out:      e.out,
		Err:      e.err,
		Getenv:   func(k string) string { return e.vars[k] },
		Getwd:    func() (string, error) { return dir, nil },
		StdinTTY: func() bool { return e.tty },
		ReadPassword: func(prompt string) ([]byte, error) {
			e.pwCalls++
			e.prompts = append(e.prompts, prompt)
			if len(e.pwQueue) == 0 {
				return nil, errors.New("unexpected password prompt")
			}
			pw := e.pwQueue[0]
			e.pwQueue = e.pwQueue[1:]
			return []byte(pw), nil
		},
		DataDir:    func() (string, error) { return dir, nil },
		NewService: func() (*app.Service, error) { return svc, nil },
	}
	return e
}

func (e *env) run(pws []string, args ...string) int {
	e.out.Reset()
	e.err.Reset()
	e.pwQueue = pws
	return Execute(e.d, args)
}

func (e *env) lookup(name string) app.VaultEntry {
	e.t.Helper()
	v, err := resolveVault(context.Background(), e.svc, name)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func TestVaultCreateAndList(t *testing.T) {
	e := newEnv(t)
	if code := e.run([]string{"pw1", "pw1"}, "vault", "create", "work"); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if e.out.String() != "created vault work\nnext: secrio unlock work\n" {
		t.Fatalf("out=%q", e.out)
	}
	if strings.Join(e.prompts, "|") != "Password: |Confirm password: " {
		t.Fatalf("prompts=%v", e.prompts)
	}
	e.vars["SECRIO_PASSWORD"] = "pw2"
	if code := e.run(nil, "vault", "create", "alpha"); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if code := e.run(nil, "vault", "list"); code != ExitOK || e.out.String() != "alpha\nwork\n" {
		t.Fatalf("code=%d out=%q", code, e.out)
	}
}

func TestVaultCreateDuplicateBeforePrompt(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "Work")
	for name, want := range map[string]string{
		"Work": "secrio: vault \"Work\" already exists\n",
		"work": "secrio: vault \"work\" already exists as \"Work\" (vault names are compared without regard to case)\n",
	} {
		e.err.Reset()
		if code := e.run([]string{"pw", "pw"}, "vault", "create", name); code != ExitFailure || e.err.String() != want {
			t.Errorf("%s: code=%d err=%q", name, code, e.err)
		}
		if e.pwCalls != 0 {
			t.Errorf("%s: prompted %d times", name, e.pwCalls)
		}
	}
}

func TestVaultListEmpty(t *testing.T) {
	e := newEnv(t)
	if code := e.run(nil, "vault", "list"); code != ExitOK || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q", code, e.out)
	}
}

func TestVaultCreateDuplicateRefused(t *testing.T) {
	e := newEnv(t)
	e.vars["SECRIO_PASSWORD"] = "pw"
	e.run(nil, "vault", "create", "work")
	if code := e.run(nil, "vault", "create", "work"); code != ExitFailure || !strings.Contains(e.err.String(), "already exists") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestVaultCreateInvalidName(t *testing.T) {
	e := newEnv(t)
	e.vars["SECRIO_PASSWORD"] = "pw"
	for _, n := range []string{"../x", "a b"} {
		if code := e.run(nil, "vault", "create", n); code != ExitFailure {
			t.Errorf("%q: code=%d", n, code)
		}
	}
	if code := e.run(nil, "vault", "list"); e.out.Len() != 0 || code != ExitOK {
		t.Fatalf("vaults created: %q", e.out)
	}
}

func TestVaultCreateConfirmMismatch(t *testing.T) {
	e := newEnv(t)
	if code := e.run([]string{"one", "two"}, "vault", "create", "work"); code != ExitFailure || !strings.Contains(e.err.String(), "passwords do not match") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	e.run(nil, "vault", "list")
	if e.out.Len() != 0 {
		t.Fatal("vault created")
	}
}

func TestVaultCreateEnvPasswordNoPrompt(t *testing.T) {
	e := newEnv(t)
	e.vars["SECRIO_PASSWORD"] = "envpw"
	e.tty = false
	if code := e.run(nil, "vault", "create", "work"); code != ExitOK || e.pwCalls != 0 {
		t.Fatalf("code=%d calls=%d err=%q", code, e.pwCalls, e.err)
	}
}

func TestVaultCreateNoTTYNoEnv(t *testing.T) {
	e := newEnv(t)
	e.tty = false
	if code := e.run([]string{"x", "x"}, "vault", "create", "work"); code != ExitLocked || e.pwCalls != 0 {
		t.Fatalf("code=%d calls=%d", code, e.pwCalls)
	}
	if !strings.Contains(e.err.String(), "no terminal and SECRIO_PASSWORD is not set") {
		t.Fatalf("err=%q", e.err)
	}
	e.run(nil, "vault", "list")
	if e.out.Len() != 0 {
		t.Fatal("vault created")
	}
}

func TestVaultCreateEmptyPassword(t *testing.T) {
	e := newEnv(t)
	if code := e.run([]string{"", ""}, "vault", "create", "work"); code != ExitFailure || !strings.Contains(e.err.String(), "password must not be empty") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	e.run(nil, "vault", "list")
	if e.out.Len() != 0 {
		t.Fatal("vault created")
	}
}

func TestVaultArgCounts(t *testing.T) {
	e := newEnv(t)
	for _, a := range [][]string{{"vault", "create"}, {"vault", "create", "a", "b"}, {"vault", "list", "x"}} {
		if code := e.run(nil, a...); code != ExitUsage {
			t.Errorf("%v: code=%d", a, code)
		}
	}
}

func TestPasswordNeverInOutput(t *testing.T) {
	const secret = "s3cr3t-never-print-me"
	e := newEnv(t)
	check := func(name string) {
		t.Helper()
		if strings.Contains(e.out.String()+e.err.String(), secret) {
			t.Errorf("%s: password in output: out=%q err=%q", name, e.out, e.err)
		}
	}
	e.run([]string{secret, secret}, "vault", "create", "work")
	check("create")
	e.run([]string{secret, "other"}, "vault", "create", "w2")
	check("mismatch")
	e.run([]string{secret, secret}, "vault", "create", "work")
	check("duplicate")
	e.run([]string{secret}, "unlock", "work", "--ttl", "0")
	check("bad ttl")
	e.run([]string{secret}, "unlock", "work")
	check("unlock")
	e.run([]string{secret}, "unlock", "nope")
	check("unknown")
	e.run([]string{secret}, "lock", "work")
	check("lock")
	e.vars["SECRIO_PASSWORD"] = secret
	e.run(nil, "unlock", "work")
	check("env unlock")
	e.tty = false
	delete(e.vars, "SECRIO_PASSWORD")
	e.run(nil, "unlock", "work")
	check("no tty")
	e.vars["SECRIO_PASSWORD"] = secret + "x"
	e.run(nil, "unlock", "work")
	check("wrong env")
}
