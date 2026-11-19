package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"secrio/internal/domain"
)

func testDeps() (Deps, *bytes.Buffer, *bytes.Buffer) {
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	return Deps{In: strings.NewReader(""), Out: out, Err: errb}, out, errb
}

func TestExecuteUnknownCommandIsUsage(t *testing.T) {
	d, _, errb := testDeps()
	if code := Execute(d, []string{"nope"}); code != ExitUsage {
		t.Fatalf("code = %d, want %d", code, ExitUsage)
	}
	if !strings.HasPrefix(errb.String(), "secrio: ") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestExecuteUnknownFlagIsUsage(t *testing.T) {
	d, _, _ := testDeps()
	if code := Execute(d, []string{"--bogus"}); code != ExitUsage {
		t.Fatalf("code = %d, want %d", code, ExitUsage)
	}
}

func TestExecuteVersion(t *testing.T) {
	d, out, _ := testDeps()
	if code := Execute(d, []string{"--version"}); code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if out.String() != "secrio dev\n" {
		t.Fatalf("out = %q", out.String())
	}
}

func TestExecuteNoArgsPrintsHelp(t *testing.T) {
	d, out, _ := testDeps()
	if code := Execute(d, nil); code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out.String(), "secrio") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestCompletionEnabled(t *testing.T) {
	d, out, _ := testDeps()
	if code := Execute(d, []string{"completion", "bash"}); code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if out.Len() == 0 {
		t.Fatal("empty completion script")
	}
}

func TestExitErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"usage", UsageErr("bad %s", "x"), ExitUsage},
		{"locked", LockedErr(errors.New("no")), ExitLocked},
		{"plain", errors.New("boom"), ExitFailure},
		{"domain locked", fmt.Errorf("wrap: %w", domain.ErrLocked), ExitLocked},
		{"domain password", fmt.Errorf("wrap: %w", domain.ErrInvalidPassword), ExitLocked},
		{"invalid argument", fmt.Errorf("wrap: %w", domain.ErrInvalidArgument), ExitUsage},
		{"explicit code", &ExitError{Code: 125, Err: errors.New("x")}, 125},
		{"wrapped exit error", fmt.Errorf("w: %w", &ExitError{Code: 125, Err: errors.New("x")}), 125},
	}
	for _, c := range cases {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("%s: code = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestErrorOutputHasPrefixAndNoStack(t *testing.T) {
	d, _, errb := testDeps()
	root := NewRoot(d)
	root.AddCommand(&cobra.Command{Use: "fail", RunE: func(*cobra.Command, []string) error {
		return errors.New("boom")
	}})
	if code := run(d, root, []string{"fail"}); code != ExitFailure {
		t.Fatalf("code = %d", code)
	}
	if errb.String() != "secrio: boom\n" {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestNoUsageDumpOnError(t *testing.T) {
	d, out, errb := testDeps()
	Execute(d, []string{"nope"})
	if strings.Contains(out.String()+errb.String(), "Usage:") {
		t.Fatalf("usage dumped: %q %q", out.String(), errb.String())
	}
	if strings.Count(errb.String(), "\n") != 1 {
		t.Fatalf("stderr not one line: %q", errb.String())
	}
}

func runWith(t *testing.T, args []string, cmds ...*cobra.Command) (int, string, string) {
	t.Helper()
	d, out, errb := testDeps()
	root := NewRoot(d)
	root.AddCommand(cmds...)
	usageArgs(root) // idempotent; covers commands added after NewRoot
	code := run(d, root, args)
	return code, out.String(), errb.String()
}

func TestArgCountErrorsAreUsage(t *testing.T) {
	mk := func() *cobra.Command {
		return &cobra.Command{Use: "one", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
	}
	for _, args := range [][]string{{"one"}, {"one", "a", "b"}} {
		code, _, errb := runWith(t, args, mk())
		if code != ExitUsage || !strings.HasPrefix(errb, "secrio: ") {
			t.Errorf("%v: code=%d stderr=%q", args, code, errb)
		}
	}
	if code, _, _ := runWith(t, []string{"one", "a"}, mk()); code != ExitOK {
		t.Errorf("valid args: code=%d", code)
	}
}

func TestGroupUnknownSubcommandIsUsage(t *testing.T) {
	grp := &cobra.Command{Use: "grp"}
	grp.AddCommand(&cobra.Command{Use: "sub", RunE: func(*cobra.Command, []string) error { return nil }})
	code, _, errb := runWith(t, []string{"grp", "bogus"}, grp)
	if code != ExitUsage || !strings.HasPrefix(errb, "secrio: ") {
		t.Fatalf("code=%d stderr=%q", code, errb)
	}
}

func TestArgsExitErrorKeepsCode(t *testing.T) {
	c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil },
		Args: func(*cobra.Command, []string) error { return LockedErr(errors.New("locked")) }}
	if code, _, _ := runWith(t, []string{"x"}, c); code != ExitLocked {
		t.Fatalf("code=%d", code)
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, f := range []string{"-h", "--help"} {
		d, out, errb := testDeps()
		if code := Execute(d, []string{f}); code != ExitOK || !strings.Contains(out.String(), "Usage:") || errb.Len() != 0 {
			t.Errorf("%s: code=%d out=%q err=%q", f, code, out.String(), errb.String())
		}
	}
}

func TestCompletionExtraArgIsUsage(t *testing.T) {
	dummy := &cobra.Command{Use: "dummy", RunE: func(*cobra.Command, []string) error { return nil }}
	if code, _, errb := runWith(t, []string{"completion", "bash", "extra"}, dummy); code != ExitUsage {
		t.Fatalf("code=%d stderr=%q", code, errb)
	}
}

func TestCompletionBashScript(t *testing.T) {
	dummy := &cobra.Command{Use: "dummy", RunE: func(*cobra.Command, []string) error { return nil }}
	if code, out, _ := runWith(t, []string{"completion", "bash"}, dummy); code != ExitOK || !strings.Contains(out, "bash") {
		t.Fatalf("code=%d", code)
	}
}

func TestHelpCommandStillWorks(t *testing.T) {
	dummy := &cobra.Command{Use: "dummy", Short: "d", RunE: func(*cobra.Command, []string) error { return nil }}
	code, out, errb := runWith(t, []string{"help", "dummy"}, dummy)
	if code != ExitOK || errb != "" || out == "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestLeafDefaultsToNoArgs(t *testing.T) {
	leaf := func(a cobra.PositionalArgs) *cobra.Command {
		return &cobra.Command{Use: "leaf", Args: a, RunE: func(*cobra.Command, []string) error { return nil }}
	}
	if code, _, _ := runWith(t, []string{"leaf", "x"}, leaf(nil)); code != ExitUsage {
		t.Errorf("nil Args with arg: code=%d", code)
	}
	if code, _, _ := runWith(t, []string{"leaf"}, leaf(nil)); code != ExitOK {
		t.Errorf("nil Args no arg: code=%d", code)
	}
	if code, _, _ := runWith(t, []string{"leaf", "x"}, leaf(cobra.ExactArgs(1))); code != ExitOK {
		t.Errorf("ExactArgs(1): code=%d", code)
	}
}

func TestUsageArgsIdempotent(t *testing.T) {
	d, _, _ := testDeps()
	root := NewRoot(d)
	leaf := &cobra.Command{Use: "leaf", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
	grp := &cobra.Command{Use: "grp"}
	grp.AddCommand(&cobra.Command{Use: "sub", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(leaf, grp)
	usageArgs(root)
	usageArgs(root)
	usageArgs(root)
	for args, want := range map[string]int{"leaf": ExitUsage, "leaf a": ExitOK, "leaf a b": ExitUsage, "grp": ExitOK, "grp x": ExitUsage, "grp sub": ExitOK, "nope": ExitUsage} {
		d, _, _ := testDeps()
		if got := run(d, root, strings.Fields(args)); got != want {
			t.Errorf("%q: code=%d want %d", args, got, want)
		}
	}
}

func TestExecuteNilArgsIgnoresOSArgs(t *testing.T) {
	old := os.Args
	os.Args = []string{"secrio", "bogus-from-os-args"}
	defer func() { os.Args = old }()
	d, _, _ := testDeps()
	if code := Execute(d, nil); code != ExitOK {
		t.Fatalf("code=%d", code)
	}
}
