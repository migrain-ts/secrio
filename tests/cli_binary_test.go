package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These tests run the real secrio binary, built once in TestMain. They never
// use unlock/lock (those go through the real OS keychain); every unlocking
// command authenticates with SECRIO_PASSWORD instead.

var secrioBin string

func TestMain(m *testing.M) { os.Exit(runMain(m)) }

func runMain(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "secrio-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(tmp)
	_, file, _, _ := runtime.Caller(0)
	secrioBin = filepath.Join(tmp, "secrio")
	if runtime.GOOS == "windows" {
		secrioBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", secrioBin, "./cmd/secrio")
	build.Dir = filepath.Dir(filepath.Dir(file)) // module root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building secrio: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

const (
	binPw    = "bin-test-password-7731"
	dbURL    = "postgres://u:p@h/db"
	binLimit = 60 * time.Second
)

type sandbox struct {
	t    *testing.T
	home string // SECRIO_HOME
	dir  string // working directory
}

func newSandbox(t *testing.T) *sandbox {
	return &sandbox{t: t, home: t.TempDir(), dir: t.TempDir()}
}

type result struct {
	out, err string
	code     int
}

// exec runs secrio with a minimal explicit environment. Entries in extra
// replace the defaults; "K=" removes K (SECRIO_PASSWORD=binPw is the default).
func (s *sandbox) exec(stdin string, extra []string, args ...string) result {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), binLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, secrioBin, args...)
	cmd.Dir = s.dir
	env := map[string]string{
		"SECRIO_HOME":     s.home,
		"SECRIO_PASSWORD": binPw,
		"HOME":            s.t.TempDir(),
		"PATH":            os.Getenv("PATH"),
	}
	for _, kv := range extra {
		k, v, _ := strings.Cut(kv, "=")
		if v == "" {
			delete(env, k)
		} else {
			env[k] = v
		}
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if stdin != "" { // otherwise stdin is /dev/null
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		s.t.Fatalf("running secrio %v: %v", args, err)
	}
	if ctx.Err() != nil {
		s.t.Fatalf("secrio %v timed out", args)
	}
	return result{out.String(), errb.String(), code}
}

func (s *sandbox) must(stdin string, args ...string) result {
	s.t.Helper()
	r := s.exec(stdin, nil, args...)
	if r.code != 0 {
		s.t.Fatalf("secrio %v: exit %d, stderr %q", args, r.code, r.err)
	}
	return r
}

// provision creates vault work > project hotenb > env production and stores
// DATABASE_URL there.
func (s *sandbox) provision() {
	s.t.Helper()
	s.must("", "vault", "create", "work")
	s.must("", "project", "create", "hotenb", "--vault", "work")
	s.must("", "env", "create", "production", "--vault", "work", "--project", "hotenb")
	s.must(dbURL, append([]string{"set", "DATABASE_URL"}, flagsWHP...)...)
}

var flagsWHP = []string{"--vault", "work", "--project", "hotenb", "--env", "production"}

func args(parts ...[]string) []string {
	var all []string
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

func skipNoSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
}

func TestCLIFullFlow(t *testing.T) {
	skipNoSh(t)
	s := newSandbox(t)
	if r := s.must("", "vault", "create", "work"); r.out != "created vault work\n" {
		t.Fatalf("vault create: %q", r.out)
	}
	s.must("", "project", "create", "hotenb", "--vault", "work")
	s.must("", "env", "create", "production", "--vault", "work", "--project", "hotenb")
	if r := s.must(dbURL, args([]string{"set", "DATABASE_URL"}, flagsWHP)...); r.out != "set DATABASE_URL\n" {
		t.Fatalf("set: %q", r.out)
	}

	if r := s.must("", "vault", "list"); r.out != "work\n" {
		t.Fatalf("vault list: %q", r.out)
	}
	if r := s.must("", "list", "--vault", "work"); r.out != "hotenb\n" {
		t.Fatalf("list projects: %q", r.out)
	}
	if r := s.must("", "list", "--vault", "work", "--project", "hotenb"); r.out != "production\n" {
		t.Fatalf("list envs: %q", r.out)
	}
	r := s.must("", args([]string{"list"}, flagsWHP)...)
	if r.out != "DATABASE_URL\n" || strings.Contains(r.out+r.err, dbURL) {
		t.Fatalf("list names: %q %q", r.out, r.err)
	}

	if r := s.must("", args([]string{"get", "DATABASE_URL"}, flagsWHP)...); r.out != dbURL {
		t.Fatalf("get: %q (want exact bytes, no newline)", r.out)
	}
	run := args([]string{"run"}, flagsWHP, []string{"--", "sh", "-c", `printf %s "$DATABASE_URL"`})
	if r := s.must("", run...); r.out != dbURL {
		t.Fatalf("run: %q", r.out)
	}
}

func TestCLIExitCodes(t *testing.T) {
	skipNoSh(t)
	s := newSandbox(t)
	s.provision()

	noPw := []string{"SECRIO_PASSWORD="}
	cases := []struct {
		name  string
		extra []string
		args  []string
		want  int
	}{
		{"unknown command", nil, []string{"nosuchcmd"}, 2},
		{"set without name", nil, []string{"set"}, 2},
		{"missing password without tty", noPw, []string{"vault", "create", "x"}, 3},
		{"locked get without password", noPw, args([]string{"get", "DATABASE_URL"}, flagsWHP), 3},
		{"child exit code", nil, args([]string{"run"}, flagsWHP, []string{"--", "sh", "-c", "exit 7"}), 7},
		{"command not found", nil, args([]string{"run"}, flagsWHP, []string{"--", "nosuchcommand_xyz"}), 127},
		{"unknown env", nil, []string{"run", "--vault", "work", "--project", "hotenb", "--env", "nope", "--", "true"}, 125},
		{"run without --", nil, args([]string{"run"}, flagsWHP, []string{"echo", "hi"}), 125},
		{"run without command", nil, args([]string{"run"}, flagsWHP, []string{"--"}), 125},
		{"run missing env context", nil, []string{"run", "--vault", "work", "--project", "hotenb", "--", "true"}, 125},
		{"run unknown flag", nil, []string{"run", "--nosuch", "--", "true"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.t = t
			start := time.Now()
			r := s.exec("", c.extra, c.args...)
			if r.code != c.want {
				t.Fatalf("exit %d, want %d (stdout %q stderr %q)", r.code, c.want, r.out, r.err)
			}
			if c.want == 3 && time.Since(start) > 10*time.Second {
				t.Fatalf("took %s, expected a fast failure", time.Since(start))
			}
		})
	}
}

func TestCLIContextFile(t *testing.T) {
	skipNoSh(t)
	s := newSandbox(t)
	s.provision()
	s.must("", "env", "create", "staging", "--vault", "work", "--project", "hotenb")
	s.must("stg-value", "set", "DATABASE_URL", "--vault", "work", "--project", "hotenb", "--env", "staging")

	proj := t.TempDir()
	toml := "vault = \"work\"\nproject = \"hotenb\"\nenv = \"production\"\n"
	if err := os.WriteFile(filepath.Join(proj, ".secrio.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s.dir = sub // proves the upward search

	if r := s.must("", "get", "DATABASE_URL"); r.out != dbURL {
		t.Fatalf("get via file: %q", r.out)
	}
	if r := s.must("", "run", "--", "sh", "-c", `printf %s "$DATABASE_URL"`); r.out != dbURL {
		t.Fatalf("run via file: %q", r.out)
	}
	if r := s.must("", "get", "DATABASE_URL", "--env", "staging"); r.out != "stg-value" {
		t.Fatalf("flag override: %q", r.out)
	}
}

func TestCLISecretNeverInArgs(t *testing.T) {
	skipNoSh(t)
	s := newSandbox(t)
	s.provision()
	base := args([]string{"run"}, flagsWHP)

	r := s.must("", args(base, []string{"--", "sh", "-c", `printf %s "$*"`, "sh", "arg1"})...)
	if r.out != "arg1" {
		t.Fatalf("child args: %q", r.out)
	}
	// The child's own command line (as ps shows it) and "$@" must not hold the value.
	r = s.must("", args(base, []string{"--", "sh", "-c", `ps -o args= -p $$ 2>/dev/null; printf "|%s" "$@"`, "sh", "arg1"})...)
	if !strings.Contains(r.out, "arg1") || strings.Contains(r.out+r.err, dbURL) {
		t.Fatalf("argv check: %q %q", r.out, r.err)
	}
}

func TestCLICompletion(t *testing.T) {
	s := newSandbox(t)
	r := s.exec("", nil, "completion", "bash")
	if r.code != 0 || r.out == "" || !strings.Contains(r.out, "secrio") {
		t.Fatalf("completion bash: exit %d, %d bytes", r.code, len(r.out))
	}
	if r := s.exec("", nil, "completion", "bash", "extra"); r.code != 2 {
		t.Fatalf("completion with extra arg: exit %d, want 2", r.code)
	}
}

func TestCLIVersion(t *testing.T) {
	s := newSandbox(t)
	if r := s.must("", "--version"); r.out != "secrio dev\n" {
		t.Fatalf("version: %q", r.out)
	}
}

func TestCLIStdinSetValueWithCRLF(t *testing.T) {
	s := newSandbox(t)
	s.provision()
	s.must("v\r\n", args([]string{"set", "K"}, flagsWHP)...)
	if r := s.must("", args([]string{"get", "K"}, flagsWHP)...); r.out != "v" {
		t.Fatalf("get: %q", r.out)
	}
}

func TestCLIEmptySetRefused(t *testing.T) {
	s := newSandbox(t)
	s.provision()
	if r := s.exec("", nil, args([]string{"set", "K"}, flagsWHP)...); r.code != 1 {
		t.Fatalf("empty set: exit %d, want 1 (stderr %q)", r.code, r.err)
	}
	if r := s.exec("", nil, args([]string{"get", "K"}, flagsWHP)...); r.code != 1 {
		t.Fatalf("get after refused set: exit %d, want 1", r.code)
	}
}

func TestCLIErrorsNeverContainPassword(t *testing.T) {
	s := newSandbox(t)
	s.provision()
	wrong := "wrong-" + binPw
	runs := []struct {
		extra []string
		args  []string
	}{
		{nil, args([]string{"get", "MISSING"}, flagsWHP)},
		{nil, []string{"get", "DATABASE_URL", "--vault", "nope", "--project", "p", "--env", "e"}},
		{nil, []string{"vault", "create", "work"}},
		{nil, []string{"set"}},
		{[]string{"SECRIO_PASSWORD=" + wrong}, args([]string{"get", "DATABASE_URL"}, flagsWHP)},
		{[]string{"SECRIO_PASSWORD="}, args([]string{"get", "DATABASE_URL"}, flagsWHP)},
	}
	for _, c := range runs {
		r := s.exec("", c.extra, c.args...)
		if r.code == 0 {
			t.Fatalf("secrio %v unexpectedly succeeded", c.args)
		}
		all := r.out + r.err
		for _, secret := range []string{binPw, dbURL, wrong} {
			if strings.Contains(all, secret) {
				t.Fatalf("secrio %v leaked a secret: %q", c.args, all)
			}
		}
	}
}
