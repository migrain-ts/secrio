package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// secretsEnv has an unlocked vault "work" with project a / env dev, a
// non-TTY stdin and a non-TTY stdout.
func secretsEnv(t *testing.T) *env {
	t.Helper()
	e := unlockedEnv(t)
	e.run(nil, "--vault", "work", "project", "create", "a")
	e.run(nil, "--vault", "work", "--project", "a", "env", "create", "dev")
	e.tty = false
	e.d.StdoutTTY = func() bool { return false }
	e.pwCalls, e.prompts = 0, nil
	return e
}

var ctxArgs = []string{"--vault", "work", "--project", "a", "--env", "dev"}

func withCtx(args ...string) []string {
	return append(append([]string{}, ctxArgs...), args...)
}

// secret runs args in the dev context with stdin as piped input.
func (e *env) secret(stdin []byte, args ...string) int {
	e.d.In = bytes.NewReader(stdin)
	return e.run(nil, withCtx(args...)...)
}

func TestSetGetRoundTrip(t *testing.T) {
	e := secretsEnv(t)
	big := strings.Repeat("A", 1<<20)
	cases := []struct{ name, in, want string }{
		{"plain", "abc", "abc"},
		{"equals", "a=b", "a=b"},
		{"multiline", "line1\nline2", "line1\nline2"},
		{"utf8", "é", "é"},
		{"non-utf8", "x\xff\xfey", "x\xff\xfey"},
		{"1MiB", big, big},
		{"internal crlf", "a\r\nb", "a\r\nb"},
		{"trailing lf", "abc\n", "abc"},
		{"trailing crlf", "abc\r\n", "abc"},
		{"trailing two lf", "abc\n\n", "abc\n"},
		{"trailing crlf crlf", "abc\r\n\r\n", "abc\r\n"},
		{"1MiB with lf", big + "\n", big},
		{"1MiB with crlf", big + "\r\n", big},
	}
	for _, c := range cases {
		if code := e.secret([]byte(c.in), "set", "K"); code != ExitOK || e.out.String() != "set K\n" {
			t.Fatalf("%s: set code=%d out=%q err=%.200q", c.name, code, e.out, e.err)
		}
		if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != c.want {
			t.Fatalf("%s: get code=%d err=%.200q (got %d bytes, want %d)", c.name, code, e.err, e.out.Len(), len(c.want))
		}
	}
}

func TestSetOverwrite(t *testing.T) {
	e := secretsEnv(t)
	e.secret([]byte("one"), "set", "K")
	e.secret([]byte("two"), "set", "K")
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "two" {
		t.Fatalf("code=%d out=%q", code, e.out)
	}
}

func TestSetRefusals(t *testing.T) {
	e := secretsEnv(t)
	cases := []struct {
		name  string
		in    []byte
		wantE string
	}{
		{"too large", []byte(strings.Repeat("A", 1<<20+1)), "value too large (max 1 MiB)"},
		{"too large with lf", []byte(strings.Repeat("A", 1<<20+1) + "\n"), "value too large (max 1 MiB)"},
		{"empty", nil, "refusing to store an empty value"},
		{"lf only", []byte("\n"), "refusing to store an empty value"},
		{"crlf only", []byte("\r\n"), "refusing to store an empty value"},
		{"NUL inside", []byte("a\x00b"), "refusing to store a value containing NUL bytes (type text)"},
		{"NUL only", []byte("\x00"), "refusing to store a value containing NUL bytes (type text)"},
		{"NUL before lf", []byte("ab\x00\n"), "refusing to store a value containing NUL bytes (type text)"},
	}
	for _, c := range cases {
		if code := e.secret(c.in, "set", "K"); code != ExitFailure || !strings.Contains(e.err.String(), c.wantE) || e.out.Len() != 0 {
			t.Errorf("%s: code=%d out=%q err=%.200q", c.name, code, e.out, e.err)
		}
		if code := e.secret(nil, "get", "K"); code != ExitFailure {
			t.Errorf("%s: something was stored (get code=%d)", c.name, code)
		}
	}
}

func TestSetTTYRefusesNUL(t *testing.T) {
	e := secretsEnv(t)
	e.tty = true
	if code := e.run([]string{"a\x00b"}, withCtx("set", "K")...); code != ExitFailure ||
		!strings.Contains(e.err.String(), "refusing to store a value containing NUL bytes (type text)") {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	e.tty = false
	if code := e.secret(nil, "get", "K"); code != ExitFailure {
		t.Fatalf("something was stored (get code=%d)", code)
	}
}

func TestSetTTYUsesReadPasswordOnce(t *testing.T) {
	e := secretsEnv(t)
	e.tty = true
	e.vars["SECRIO_PASSWORD"] = "vaultpw"
	if code := e.run([]string{"v1"}, withCtx("set", "K")...); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if e.pwCalls != 1 || len(e.prompts) != 1 || e.prompts[0] != "Value: " {
		t.Fatalf("calls=%d prompts=%q", e.pwCalls, e.prompts)
	}
	if e.out.String() != "set K\n" || e.err.Len() != 0 {
		t.Fatalf("out=%q err=%q", e.out, e.err)
	}
	e.tty = false
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "v1" {
		t.Fatalf("get code=%d out=%q", code, e.out)
	}
}

func TestSetTTYValueNotStripped(t *testing.T) {
	e := secretsEnv(t)
	e.tty = true
	e.run([]string{"v1\n"}, withCtx("set", "K")...)
	e.tty = false
	if e.secret(nil, "get", "K"); e.out.String() != "v1\n" {
		t.Fatalf("out=%q", e.out)
	}
}

func TestSetNoTTYEOF(t *testing.T) {
	e := secretsEnv(t)
	e.d.In = strings.NewReader("")
	if code := e.run(nil, withCtx("set", "K")...); code != ExitFailure {
		t.Fatalf("code=%d", code)
	}
	if e.pwCalls != 0 {
		t.Fatal("prompted without a TTY")
	}
}

func TestGetNewlineOnlyOnTTY(t *testing.T) {
	e := secretsEnv(t)
	e.secret([]byte("abc"), "set", "K")
	e.d.StdoutTTY = func() bool { return true }
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "abc\n" {
		t.Fatalf("code=%d out=%q", code, e.out)
	}
}

func TestGetUnknown(t *testing.T) {
	e := secretsEnv(t)
	if code := e.secret(nil, "get", "NOPE"); code != ExitFailure || !strings.Contains(e.err.String(), "secret not found") || e.out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
}

type failReader struct{ t *testing.T }

func (r failReader) Read([]byte) (int, error) {
	r.t.Error("value was read")
	return 0, io.EOF
}

func TestInvalidName(t *testing.T) {
	e := secretsEnv(t)
	for _, n := range []string{"1BAD", "a-b"} {
		e.d.In = failReader{t}
		if code := e.run(nil, withCtx("set", n)...); code != ExitFailure {
			t.Errorf("set %s: code=%d err=%q", n, code, e.err)
		}
		if code := e.secret(nil, "get", n); code != ExitFailure {
			t.Errorf("get %s: code=%d err=%q", n, code, e.err)
		}
	}
	e.tty = true
	if e.run(nil, withCtx("set", "1BAD")...); e.pwCalls != 0 {
		t.Error("prompted for an invalid name")
	}
}

func TestSecretsUsageErrors(t *testing.T) {
	e := secretsEnv(t)
	for _, args := range [][]string{
		{"set", "A", "B"},
		{"set"},
		{"set", "A", "--value", "x"},
		{"get"},
		{"get", "A", "B"},
	} {
		if code := e.secret([]byte("x"), args...); code != ExitUsage {
			t.Errorf("%v: code=%d err=%q", args, code, e.err)
		}
	}
}

func TestSecretsMissingContextBeforeUnlock(t *testing.T) {
	e := secretsEnv(t)
	e.run(nil, "lock", "work")
	for _, cmd := range []string{"set", "get"} {
		for _, args := range [][]string{
			{"--vault", "work", "--project", "a", cmd, "K"},
			{"--vault", "work", "--env", "dev", cmd, "K"},
			{"--project", "a", "--env", "dev", cmd, "K"},
		} {
			e.d.In = strings.NewReader("x")
			if code := e.run(nil, args...); code != ExitUsage {
				t.Errorf("%v: code=%d err=%q", args, code, e.err)
			}
		}
	}
}

func TestSecretsLockedAndEnvPassword(t *testing.T) {
	e := secretsEnv(t)
	e.run(nil, "lock", "work")
	for _, args := range [][]string{{"set", "K"}, {"get", "K"}} {
		if code := e.secret([]byte("x"), args...); code != ExitLocked {
			t.Fatalf("%v: code=%d err=%q", args, code, e.err)
		}
	}
	e.vars["SECRIO_PASSWORD"] = testPassword
	if code := e.secret([]byte("val"), "set", "K"); code != ExitOK {
		t.Fatalf("set code=%d err=%q", code, e.err)
	}
	if code := e.secret(nil, "get", "K"); code != ExitOK || e.out.String() != "val" {
		t.Fatalf("get code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestNoValueLeak(t *testing.T) {
	const v = "TOPSECRET-value-123"
	e := secretsEnv(t)
	check := func(label string) {
		t.Helper()
		if strings.Contains(e.out.String()+e.err.String(), v) {
			t.Errorf("%s: value leaked: out=%q err=%q", label, e.out, e.err)
		}
	}
	e.secret([]byte(v), "set", "1BAD")
	check("invalid name")
	e.secret([]byte(v), "set", "A", "B")
	check("two args")
	e.secret([]byte(v), "set", "A", "--value", v)
	check("--value")
	e.secret([]byte(v+strings.Repeat("A", 1<<20)), "set", "K")
	check("too large")
	e.secret([]byte(v), "get", "NOPE")
	check("unknown")
	e.secret([]byte(v), "--env", "", "set", "K")
	check("missing env")
	e.run(nil, "lock", "work")
	e.secret([]byte(v), "set", "K")
	check("locked")
}
