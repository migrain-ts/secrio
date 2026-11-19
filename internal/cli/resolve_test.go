package cli

import (
	"context"
	"go/build"
	"path/filepath"
	"strings"
	"testing"

	"secrio/internal/core"
)

func TestValidVaultName(t *testing.T) {
	for _, ok := range []string{"work", "a.b-c_d", "A1", "a-b", "x-", strings.Repeat("a", 64)} {
		if err := validVaultName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, ok := range []string{"com0", "com10", "console", "lpt", "nulx", "a.con", "my-aux"} {
		if err := validVaultName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	bads := []string{"", "a/b", "..", "a b", strings.Repeat("a", 65), "é", "a\nb",
		"a\n", "-x", "--v", "-", "a٣", "١٢", "a..b", ".hidden", ".", "...",
		"con", "CON", "Prn", "aux", "NUL", "com1", "COM9", "lpt1", "Lpt9",
		"con.txt", "NUL.db", "com3.x.y", "con."}
	dir := t.TempDir()
	if _, err := core.VaultPath(dir, "a..b"); err == nil {
		t.Error("core.VaultPath accepts a..b: CLI and Core disagree")
	}
	for _, bad := range bads {
		err := validVaultName(bad)
		if err == nil {
			t.Errorf("%q accepted", bad)
			continue
		}
		if !strings.Contains(err.Error(), "invalid vault name") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestResolvers(t *testing.T) {
	ctx := context.Background()
	svc, dir, _ := newTestCore(t)
	id := createVault(t, svc, dir, "work")
	u := openUnlocked(t, svc, id)

	e, err := resolveVault(ctx, svc, "work")
	if err != nil || e.ID != id || e.Name != "work" {
		t.Fatalf("resolveVault = %+v, %v", e, err)
	}
	if _, err := resolveVault(ctx, svc, "nope"); err == nil || err.Error() != `unknown vault "nope"` {
		t.Fatalf("got %v", err)
	}

	p, err := u.CreateProject(ctx, "api", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := u.CreateEnvironment(ctx, p.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	gp, err := resolveProject(ctx, u, "api")
	if err != nil || gp.ID != p.ID {
		t.Fatalf("resolveProject = %+v, %v", gp, err)
	}
	if _, err := resolveProject(ctx, u, "x"); err == nil || err.Error() != `unknown project "x"` {
		t.Fatalf("got %v", err)
	}
	ge, err := resolveEnv(ctx, u, p.ID, "dev")
	if err != nil || ge.ID != env.ID {
		t.Fatalf("resolveEnv = %+v, %v", ge, err)
	}
	if _, err := resolveEnv(ctx, u, p.ID, "prod"); err == nil || err.Error() != `unknown environment "prod"` {
		t.Fatalf("got %v", err)
	}
}

func TestResolveVaultAmbiguous(t *testing.T) {
	ctx := context.Background()
	svc, dir, _ := newTestCore(t)
	createVault(t, svc, dir, "dup")
	// The Core may not forbid it; the CLI must not pick one silently.
	if _, err := svc.CreateVault(ctx, "dup", filepath.Join(dir, "dup2.db"), []byte(testPassword)); err != nil {
		t.Skipf("core refuses duplicate names: %v", err)
	}
	_, err := resolveVault(ctx, svc, "dup")
	if err == nil || err.Error() != `ambiguous vault name "dup"` {
		t.Fatalf("got %v", err)
	}
}

func TestNoDirectInfraImport(t *testing.T) {
	for _, dir := range []string{".", "../../cmd/secrio"} {
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp, "secrio/internal/infra") {
				t.Errorf("%s imports %s", dir, imp)
			}
		}
	}
}
