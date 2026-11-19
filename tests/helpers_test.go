package tests

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	_ "modernc.org/sqlite"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
)

var cheapKDF = domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 1}

const (
	testPw = "correct horse battery staple"
	hour   = time.Hour
)

// env is a fully provisioned vault: one project with two environments.
type env struct {
	dir      string
	svc      *app.Service
	vaultID  string
	dbPath   string
	proj     domain.Project
	dev, prd domain.Environment
}

// newEnv creates a service on a fresh dir (mock keyring), a vault, a project
// and two environments. The vault is left locked.
func newEnv(t *testing.T, opts ...core.Option) *env {
	t.Helper()
	keyring.MockInit()
	return newEnvIn(t, t.TempDir(), opts...)
}

func newEnvIn(t *testing.T, dir string, opts ...core.Option) *env {
	t.Helper()
	ctx := context.Background()
	svc, err := core.New(dir, append([]core.Option{core.WithKDFParams(cheapKDF)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	p, err := core.VaultPath(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.CreateVault(ctx, "My Vault", p, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{dir: dir, svc: svc, vaultID: string(v.ID), dbPath: p}
	e.unlock(t)
	u := e.open(t)
	defer u.Close()
	if e.proj, err = u.CreateProject(ctx, "payments-project", "payments-description"); err != nil {
		t.Fatal(err)
	}
	if e.dev, err = u.CreateEnvironment(ctx, e.proj.ID, "dev-env"); err != nil {
		t.Fatal(err)
	}
	if e.prd, err = u.CreateEnvironment(ctx, e.proj.ID, "prod-env"); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) unlock(t *testing.T) {
	t.Helper()
	if err := e.svc.Unlock(context.Background(), e.vaultID, []byte(testPw), hour, 0); err != nil {
		t.Fatal(err)
	}
}

func (e *env) open(t *testing.T) *app.Unlocked {
	t.Helper()
	u, err := e.svc.Open(context.Background(), e.vaultID)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// rawDB opens the vault file directly with SQL. The vault must be closed.
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
