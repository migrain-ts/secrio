//go:build !windows

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// runEnv is secretsEnv with the real runner and a controlled parent
// environment (PATH, PARENT_VAR, MY_OVERRIDE, SECRIO_PASSWORD).
func runEnv(t *testing.T) *env {
	t.Helper()
	e := secretsEnv(t)
	e.d.Run = systemRunner(nil)
	e.d.Environ = func() []string {
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"PARENT_VAR=keep me",
			"MY_OVERRIDE=from parent",
			"SECRIO_PASSWORD=" + testPassword,
		}
	}
	return e
}

func (e *env) runCmd(args ...string) int {
	e.d.In = strings.NewReader("")
	return e.run(nil, withCtx(append([]string{"run", "--"}, args...)...)...)
}

func TestRunSecretReachesChild(t *testing.T) {
	e := runEnv(t)
	for _, v := range []string{"plain", "a=b=c", "line1\nline2"} {
		e.secret([]byte(v), "set", "MY_SECRET")
		if code := e.runCmd("sh", "-c", `printf %s "$MY_SECRET"`); code != ExitOK || e.out.String() != v || e.err.Len() != 0 {
			t.Fatalf("%q: code=%d out=%q err=%q", v, code, e.out, e.err)
		}
	}
}

func TestRunParentVarsAndOverride(t *testing.T) {
	e := runEnv(t)
	e.secret([]byte("from vault"), "set", "MY_OVERRIDE")
	if code := e.runCmd("sh", "-c", `printf '%s|%s' "$PARENT_VAR" "$MY_OVERRIDE"`); code != ExitOK {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
	if e.out.String() != "keep me|from vault" {
		t.Fatalf("out=%q", e.out)
	}
}

func TestRunPasswordNotInChild(t *testing.T) {
	e := runEnv(t)
	e.run(nil, "lock", "work")
	e.vars["SECRIO_PASSWORD"] = testPassword // vault opened via the password
	if code := e.runCmd("sh", "-c", `printf %s "${SECRIO_PASSWORD-unset}"`); code != ExitOK || e.out.String() != "unset" {
		t.Fatalf("code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestRunExitCodes(t *testing.T) {
	e := runEnv(t)
	noexec := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(noexec, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		args    []string
		want    int
		errText string
	}{
		{"exit 7", []string{"sh", "-c", "exit 7"}, 7, ""},
		{"killed by TERM", []string{"sh", "-c", "kill -TERM $$"}, 143, ""},
		{"not found", []string{"nosuchcommand_xyz"}, 127, "secrio: "},
		{"not found path", []string{filepath.Join(t.TempDir(), "nope")}, 127, "secrio: "},
		{"not executable", []string{noexec}, 126, "secrio: "},
	}
	for _, c := range cases {
		code := e.runCmd(c.args...)
		if code != c.want {
			t.Errorf("%s: code=%d want %d err=%q", c.name, code, c.want, e.err)
		}
		if c.errText == "" && e.err.Len() != 0 {
			t.Errorf("%s: secrio printed %q", c.name, e.err)
		}
		if c.errText != "" && !strings.HasPrefix(e.err.String(), c.errText) {
			t.Errorf("%s: err=%q", c.name, e.err)
		}
	}
}

func TestRunSecrioFailures125(t *testing.T) {
	const v = "TOPSECRET-value-123"
	e := runEnv(t)
	e.secret([]byte(v), "set", "K")

	if code := e.run(nil, "--vault", "work", "--project", "a", "--env", "nope", "run", "--", "true"); code != 125 || !strings.Contains(e.err.String(), "unknown environment") {
		t.Errorf("unknown env: code=%d err=%q", code, e.err)
	}
	if code := e.run(nil, "--vault", "work", "--project", "zz", "--env", "dev", "run", "--", "true"); code != 125 {
		t.Errorf("unknown project: code=%d err=%q", code, e.err)
	}
	if code := e.run(nil, "--vault", "nov", "--project", "a", "--env", "dev", "run", "--", "true"); code != 125 {
		t.Errorf("unknown vault: code=%d err=%q", code, e.err)
	}

	e.secret([]byte("a\x00"+v), "set", "NUL")
	if code := e.runCmd("sh", "-c", "echo ran"); code != 125 || e.out.Len() != 0 || strings.Contains(e.err.String(), v) {
		t.Errorf("NUL: code=%d out=%q err=%q", code, e.out, e.err)
	}

	e.run(nil, "lock", "work")
	if code := e.runCmd("true"); code != 125 || !strings.Contains(e.err.String(), "secrio unlock") || strings.Contains(e.err.String(), v) {
		t.Errorf("locked: code=%d err=%q", code, e.err)
	}
}

func TestRunUsageErrors(t *testing.T) {
	e := runEnv(t)
	e.run(nil, "lock", "work") // usage errors win over a locked vault
	for _, args := range [][]string{
		withCtx("run"),
		withCtx("run", "sh", "-c", "true"),
		withCtx("run", "--"),
		withCtx("run", "x", "--", "true"),
		{"--vault", "work", "--env", "dev", "run", "--", "true"},
		{"--vault", "work", "--project", "a", "run", "--", "true"},
		{"--project", "a", "--env", "dev", "run", "--", "true"},
	} {
		if code := e.run(nil, args...); code != ExitUsage {
			t.Errorf("%v: code=%d err=%q", args, code, e.err)
		}
	}
}

func TestRunPassthrough(t *testing.T) {
	e := runEnv(t)
	if code := e.runCmd("sh", "-c", `printf 'out\001\n\nx'; printf 'err\002\n' >&2`); code != ExitOK {
		t.Fatalf("code=%d", code)
	}
	if e.out.String() != "out\x01\n\nx" || e.err.String() != "err\x02\n" {
		t.Fatalf("out=%q err=%q", e.out, e.err)
	}
	e.out.Reset()
	e.err.Reset()
	e.d.In = strings.NewReader("hello\nworld")
	if code := Execute(e.d, withCtx("run", "--", "cat")); code != ExitOK || e.out.String() != "hello\nworld" {
		t.Fatalf("stdin: code=%d out=%q err=%q", code, e.out, e.err)
	}
}

func TestRunRecordingRunner(t *testing.T) {
	const v = "TOPSECRET-value-123"
	e := runEnv(t)
	e.secret([]byte(v), "set", "MY_SECRET")

	var opened *app.Unlocked
	testHookRunVault = func(u *app.Unlocked) { opened = u }
	t.Cleanup(func() { testHookRunVault = nil })

	called := false
	e.d.Run = func(ctx context.Context, name string, args, env []string, in io.Reader, out, errw io.Writer) (int, error) {
		called = true
		if name != "mycmd" || strings.Join(args, "|") != "--flag|"+"$MY_SECRET" {
			t.Errorf("name=%q args=%q", name, args)
		}
		if strings.Contains(name+strings.Join(args, ""), v) {
			t.Error("value in name/args")
		}
		if !contains(env, "MY_SECRET="+v) || !contains(env, "PARENT_VAR=keep me") {
			t.Errorf("env missing entries")
		}
		for _, kv := range env {
			if strings.HasPrefix(kv, "SECRIO_PASSWORD=") {
				t.Error("SECRIO_PASSWORD reached the child env")
			}
		}
		if opened == nil {
			t.Error("hook not called")
		} else if _, err := opened.ListProjects(ctx); !errors.Is(err, domain.ErrLocked) {
			t.Errorf("vault handle still open when the runner was invoked: %v", err)
		}
		if in != e.d.In || out != e.d.Out || errw != e.d.Err {
			t.Error("stdio not passed through")
		}
		return 0, nil
	}
	if code := e.runCmd("mycmd", "--flag", "$MY_SECRET"); code != ExitOK || !called {
		t.Fatalf("code=%d called=%v err=%q", code, called, e.err)
	}
	if strings.Contains(e.out.String()+e.err.String(), v) {
		t.Fatal("value printed")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// readyWriter closes ready on the first write.
type readyWriter struct {
	once  sync.Once
	ready chan struct{}
	mu    sync.Mutex
	buf   bytes.Buffer
}

func (w *readyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.once.Do(func() { close(w.ready) })
	return w.buf.Write(p)
}

func TestRunnerRelaysSignal(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	w := &readyWriter{ready: make(chan struct{})}
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := systemRunner(sigs)(context.Background(), "sh",
			[]string{"-c", `trap "exit 9" TERM; echo ready; while :; do sleep 0.1; done`},
			[]string{"PATH=" + os.Getenv("PATH")}, strings.NewReader(""), w, io.Discard)
		done <- result{code, err}
	}()
	timeout := time.After(5 * time.Second)
	select {
	case <-w.ready:
	case r := <-done:
		t.Fatalf("child exited early: %+v", r)
	case <-timeout:
		t.Fatal("child never became ready")
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case sigs <- syscall.SIGTERM:
		default:
		}
		select {
		case r := <-done:
			if r.code != 9 || r.err != nil {
				t.Fatalf("code=%d err=%v", r.code, r.err)
			}
			return
		case <-tick.C:
		case <-timeout:
			t.Fatal("child did not exit after SIGTERM")
		}
	}
}
