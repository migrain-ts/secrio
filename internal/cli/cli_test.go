package cli

import (
	"bytes"
	"errors"
	"fmt"
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
