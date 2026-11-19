package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// simulateOwner makes the ownership check see uid as every file's owner and
// euid as the running user, restoring the real checks afterwards.
func simulateOwner(t *testing.T, uid uint32, euid int) {
	t.Helper()
	oldOwner, oldEUID := fileOwnerUID, effectiveUID
	fileOwnerUID = func(os.FileInfo) (uint32, bool) { return uid, true }
	effectiveUID = func() int { return euid }
	t.Cleanup(func() { fileOwnerUID, effectiveUID = oldOwner, oldEUID })
}

func TestLoadContextRefusesOtherOwner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrio.toml")
	writeFile(t, path, "vault = \"v\"\n")
	simulateOwner(t, 4242, 1000)
	_, err := LoadContext(ctxDeps(dir, nil), Flags{})
	want := "refusing to use " + path + ": owned by another user (pass --vault/--project/--env or SECRIO_* variables instead, or fix the ownership)"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		t.Fatalf("must be a plain error (exit 1), got code %d", ee.Code)
	}
	// Flags/env covering every field never read the file.
	full := Flags{Vault: "a", Project: "b", Env: "c"}
	if c, err := LoadContext(ctxDeps(dir, nil), full); err != nil || c.Vault != "a" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadContextOwnerAllowed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".secrio.toml"), "vault = \"v\"\n")
	// Same user.
	simulateOwner(t, 1000, 1000)
	if c, err := LoadContext(ctxDeps(dir, nil), Flags{}); err != nil || c.Vault != "v" {
		t.Fatalf("same user: %+v, %v", c, err)
	}
	// Root may use a file owned by anyone.
	simulateOwner(t, 4242, 0)
	if c, err := LoadContext(ctxDeps(dir, nil), Flags{}); err != nil || c.Vault != "v" {
		t.Fatalf("root: %+v, %v", c, err)
	}
	// No ownership information (e.g. Windows): no check.
	fileOwnerUID = func(os.FileInfo) (uint32, bool) { return 0, false }
	effectiveUID = func() int { return 1000 }
	if c, err := LoadContext(ctxDeps(dir, nil), Flags{}); err != nil || c.Vault != "v" {
		t.Fatalf("unknown owner: %+v, %v", c, err)
	}
}

func TestLoadContextRealOwnerPasses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".secrio.toml"), "vault = \"v\"\n")
	if c, err := LoadContext(ctxDeps(dir, nil), Flags{}); err != nil || c.Vault != "v" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadContextDirectoryIgnored(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, ".secrio.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".secrio.toml"), "vault = \"up\"\n")
	c, err := LoadContext(ctxDeps(sub, nil), Flags{})
	if err != nil || c.Vault != "up" {
		t.Fatalf("directory must be skipped: %+v, %v", c, err)
	}
	// Only a directory, no file anywhere: no context, no error.
	only := t.TempDir()
	if err := os.Mkdir(filepath.Join(only, ".secrio.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadContext(ctxDeps(only, nil), Flags{}); err != nil || c != (Context{}) {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadContextUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a non-root Unix user")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrio.toml")
	writeFile(t, path, "vault = \"hunter2-secret\"\n")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	_, err := LoadContext(ctxDeps(dir, nil), Flags{})
	if err == nil || !strings.Contains(err.Error(), path) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks file content: %v", err)
	}
}

func TestLoadContextEmptyFlagIsUnset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".secrio.toml"), "vault = \"fv\"\nproject = \"fp\"\n")
	c, err := LoadContext(ctxDeps(dir, map[string]string{"SECRIO_PROJECT": "ep"}), Flags{Vault: "", Project: ""})
	if err != nil || c.Vault != "fv" || c.Project != "ep" {
		t.Fatalf("got %+v, %v", c, err)
	}
	// Through the real flag parser: --vault '' is unset too.
	e := newEnv(t)
	writeFile(t, filepath.Join(e.dir, ".secrio.toml"), "vault = \"nosuch\"\n")
	if code := e.run(nil, "list", "--vault", ""); code != ExitFailure || !strings.Contains(e.err.String(), `unknown vault "nosuch"`) {
		t.Fatalf("code=%d err=%q", code, e.err)
	}
}
