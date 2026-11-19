package cli

import (
	"context"
	"strings"
	"testing"

	"secrio/internal/domain"
)

// unlockedEnv returns an env with vault "work" created and unlocked.
func unlockedEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	mkVault(t, e, "work")
	if code := e.run([]string{testPassword}, "unlock", "work"); code != ExitOK {
		t.Fatalf("unlock code=%d err=%q", code, e.err)
	}
	return e
}

func TestProjectAndEnvCreate(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "project", "create", "api"); code != ExitOK || e.out.String() != "created project api\n" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if code := e.run(nil, "--vault", "work", "--project", "api", "env", "create", "dev"); code != ExitOK || e.out.String() != "created environment dev\n" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestProjectCreateDuplicate(t *testing.T) {
	e := unlockedEnv(t)
	e.run(nil, "--vault", "work", "project", "create", "api")
	if code := e.run(nil, "--vault", "work", "project", "create", "api"); code != ExitFailure || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if e.err.String() != "secrio: project \"api\" already exists\n" {
		t.Fatalf("err=%q", e.err)
	}
}

func TestEnvCreateDuplicate(t *testing.T) {
	e := unlockedEnv(t)
	e.run(nil, "--vault", "work", "project", "create", "api")
	e.run(nil, "--vault", "work", "--project", "api", "env", "create", "dev")
	if code := e.run(nil, "--vault", "work", "--project", "api", "env", "create", "dev"); code != ExitFailure {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if e.err.String() != "secrio: environment \"dev\" already exists\n" {
		t.Fatalf("err=%q", e.err)
	}
}

func TestEnvCreateNeedsProject(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "env", "create", "dev"); code != ExitUsage || !strings.Contains(e.err.String(), "project") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestEnvCreateUnknownProject(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "--project", "nope", "env", "create", "dev"); code != ExitFailure || !strings.Contains(e.err.String(), `unknown project "nope"`) {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestStructureNeedsVaultAndLocked(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "project", "create", "api"); code != ExitUsage {
		t.Fatalf("no vault code=%d", code)
	}
	e.run(nil, "lock", "work")
	if code := e.run(nil, "--vault", "work", "project", "create", "api"); code != ExitLocked {
		t.Fatalf("locked code=%d err=%q", code, e.err)
	}
}

func TestStructureArgs(t *testing.T) {
	e := unlockedEnv(t)
	for _, args := range [][]string{
		{"project", "create"}, {"project", "create", "a", "b"},
		{"env", "create"}, {"env", "create", "a", "b"},
	} {
		full := append([]string{"--vault", "work", "--project", "api"}, args...)
		if code := e.run(nil, full...); code != ExitUsage {
			t.Errorf("%v: code=%d", args, code)
		}
	}
}

func TestStructureEnvPasswordKeepsLocked(t *testing.T) {
	e := newEnv(t)
	mkVault(t, e, "work")
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.run(nil, "--vault", "work", "project", "create", "api"); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if !isLocked(t, e, "work") {
		t.Fatal("vault left unlocked")
	}
}

// seedSecret sets a secret through the Core.
func seedSecret(t *testing.T, e *env, project, envName, secret, value string) {
	t.Helper()
	ctx := context.Background()
	u, err := openVault(ctx, e.d, e.svc, e.lookup("work"))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	p, err := resolveProject(ctx, u, project)
	if err != nil {
		t.Fatal(err)
	}
	en, err := resolveEnv(ctx, u, p.ID, envName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.SetSecret(ctx, en.ID, domain.SecretName(secret), []byte(value), "", ""); err != nil {
		t.Fatal(err)
	}
}
