package cli

import (
	"strings"
	"testing"
)

func TestListDepths(t *testing.T) {
	e := unlockedEnv(t)
	for _, p := range []string{"b", "a", "c"} {
		e.run(nil, "--vault", "work", "project", "create", p)
	}
	for _, n := range []string{"prod", "dev", "stg"} {
		e.run(nil, "--vault", "work", "--project", "a", "env", "create", n)
	}
	const secretVal = "s3cr3t-VALUE-xyz"
	seedSecret(t, e, "a", "dev", "ZED", secretVal)
	seedSecret(t, e, "a", "dev", "ALPHA", secretVal)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--vault", "work", "list"}, "a\nb\nc\n"},
		{[]string{"--vault", "work", "--project", "a", "list"}, "dev\nprod\nstg\n"},
		{[]string{"--vault", "work", "--project", "a", "--env", "dev", "list"}, "ALPHA\nZED\n"},
		{[]string{"--vault", "work", "--project", "a", "--env", "prod", "list"}, ""},
	}
	for _, c := range cases {
		if code := e.run(nil, c.args...); code != ExitOK || e.out.String() != c.want {
			t.Errorf("%v: code=%d out=%q err=%q", c.args, code, e.out, e.err)
		}
		if strings.Contains(e.out.String()+e.err.String(), secretVal) {
			t.Errorf("%v: value leaked", c.args)
		}
	}
}

func TestListEmptyVault(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "list"); code != ExitOK || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q", code, e.out)
	}
}

func TestListEnvWithoutProject(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "--env", "dev", "list"); code != ExitUsage || !strings.Contains(e.err.String(), "--project") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestListUnknownNames(t *testing.T) {
	e := unlockedEnv(t)
	e.run(nil, "--vault", "work", "project", "create", "a")
	if code := e.run(nil, "--vault", "work", "--project", "nope", "list"); code != ExitFailure || !strings.Contains(e.err.String(), `unknown project "nope"`) {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if code := e.run(nil, "--vault", "work", "--project", "a", "--env", "nope", "list"); code != ExitFailure || !strings.Contains(e.err.String(), `unknown environment "nope"`) {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}

func TestListLockedAndEnvPassword(t *testing.T) {
	e := unlockedEnv(t)
	e.run(nil, "--vault", "work", "project", "create", "a")
	e.run(nil, "lock", "work")
	if code := e.run(nil, "--vault", "work", "list"); code != ExitLocked {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.run(nil, "--vault", "work", "list"); code != ExitOK || e.out.String() != "a\n" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
	if !isLocked(t, e, "work") {
		t.Fatal("vault left unlocked")
	}
}

func TestListArgsAndNoVault(t *testing.T) {
	e := unlockedEnv(t)
	if code := e.run(nil, "--vault", "work", "list", "extra"); code != ExitUsage {
		t.Fatalf("extra code=%d", code)
	}
	if code := e.run(nil, "list"); code != ExitUsage {
		t.Fatalf("no vault code=%d", code)
	}
}
