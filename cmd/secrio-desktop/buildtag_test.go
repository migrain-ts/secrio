//go:build !desktopapp

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDesktopMainBuildTagOnly checks that the Wails shell only exists under
// the desktopapp tag, so untagged (CGO-free) builds never compile Wails.
func TestDesktopMainBuildTagOnly(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
	}
	list := func(args ...string) (string, error) {
		out, err := exec.Command(goBin, append([]string{"list"}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := list("-tags", "desktopapp", "-f", "{{len .GoFiles}}", "."); err != nil || out == "0" {
		t.Fatalf("tagged: %q, %v; want Go files", out, err)
	}
	if out, err := list("-e", "-f", "{{len .GoFiles}}", "."); err != nil || out != "0" {
		t.Fatalf("untagged: %q, %v; want no Go files", out, err)
	}
	for _, imp := range strings.Fields(mustList(t, list, "-deps", "-f", "{{.ImportPath}}", "secrio/...")) {
		if strings.HasPrefix(imp, "github.com/wailsapp") {
			t.Fatalf("untagged build imports %s", imp)
		}
	}
}

func mustList(t *testing.T, list func(...string) (string, error), args ...string) string {
	t.Helper()
	out, err := list(args...)
	if err != nil {
		t.Fatalf("go list %v: %v\n%s", args, err, out)
	}
	return out
}
