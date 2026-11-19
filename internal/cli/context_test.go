package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func ctxDeps(cwd string, env map[string]string) Deps {
	d, _, _ := testDeps()
	d.Getwd = func() (string, error) { return cwd, nil }
	d.Getenv = func(k string) string { return env[k] }
	return d
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadContextPriority(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".secrio.toml"), "vault = \"fv\"\nproject = \"fp\"\nenv = \"fe\"\n")
	env := map[string]string{"SECRIO_VAULT": "ev", "SECRIO_PROJECT": "ep"}
	c, err := LoadContext(ctxDeps(dir, env), Flags{Vault: "flagv"})
	if err != nil {
		t.Fatal(err)
	}
	want := Context{Vault: "flagv", Project: "ep", Env: "fe"}
	if c != want {
		t.Fatalf("got %+v, want %+v", c, want)
	}
}

func TestLoadContextEmptyEnvIsUnset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".secrio.toml"), "vault = \"fv\"\n")
	c, err := LoadContext(ctxDeps(dir, map[string]string{"SECRIO_VAULT": ""}), Flags{})
	if err != nil || c.Vault != "fv" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadContextParentDirNearestWins(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".secrio.toml"), "vault = \"far\"\nproject = \"farp\"\n")
	c, err := LoadContext(ctxDeps(sub, nil), Flags{})
	if err != nil || c.Vault != "far" {
		t.Fatalf("got %+v, %v", c, err)
	}
	writeFile(t, filepath.Join(root, "a", ".secrio.toml"), "vault = \"near\"\n")
	c, err = LoadContext(ctxDeps(sub, nil), Flags{})
	if err != nil || c.Vault != "near" || c.Project != "" {
		t.Fatalf("no merging expected: got %+v, %v", c, err)
	}
}

func TestLoadContextNoFileStopsAtRoot(t *testing.T) {
	c, err := LoadContext(ctxDeps(t.TempDir(), nil), Flags{})
	if err != nil || c != (Context{}) {
		t.Fatalf("got %+v, %v", c, err)
	}
	if _, err := LoadContext(ctxDeps(string(filepath.Separator), nil), Flags{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadContextMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrio.toml")
	writeFile(t, path, "vault = \"hunter2-secret\n")
	_, err := LoadContext(ctxDeps(dir, nil), Flags{})
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks file content: %v", err)
	}
}

func TestLoadContextUnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrio.toml")
	writeFile(t, path, "vault = \"v\"\nbogus = \"x\"\n")
	_, err := LoadContext(ctxDeps(dir, nil), Flags{})
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadContextWrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrio.toml")
	writeFile(t, path, "vault = 12345678\n")
	_, err := LoadContext(ctxDeps(dir, nil), Flags{})
	if err == nil || !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "12345678") {
		t.Fatalf("got %v", err)
	}
}

func TestNeed(t *testing.T) {
	c := Context{}
	for name, f := range map[string]func() (string, error){
		"vault": c.NeedVault, "project": c.NeedProject, "env": c.NeedEnv,
	} {
		_, err := f()
		var ee *ExitError
		if !errors.As(err, &ee) || ee.Code != ExitUsage {
			t.Errorf("%s: got %v", name, err)
			continue
		}
		up := strings.ToUpper(name)
		if !strings.Contains(err.Error(), "--"+name) || !strings.Contains(err.Error(), "SECRIO_"+up) || !strings.Contains(err.Error(), ".secrio.toml") {
			t.Errorf("%s: message %q", name, err)
		}
	}
	got, err := Context{Vault: "v", Project: "p", Env: "e"}.NeedVault()
	if err != nil || got != "v" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestContextFlagsPersistent(t *testing.T) {
	d, _, _ := testDeps()
	root := NewRoot(d)
	var got Flags
	root.AddCommand(&cobra.Command{Use: "dummy", RunE: func(cmd *cobra.Command, _ []string) error {
		got.Vault, _ = cmd.Flags().GetString("vault")
		got.Project, _ = cmd.Flags().GetString("project")
		got.Env, _ = cmd.Flags().GetString("env")
		return nil
	}})
	if code := run(d, root, []string{"dummy", "--vault", "v", "--project", "p", "--env", "e"}); code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if got != (Flags{"v", "p", "e"}) {
		t.Fatalf("got %+v", got)
	}
}
