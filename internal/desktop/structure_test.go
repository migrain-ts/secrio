package desktop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"secrio/internal/domain"
)

const (
	seedValue = "s3cr3t-VALUE"
	seedName  = "SEEDED_SECRET_NAME"
)

// seed sets a secret straight through the Core.
func seed(t *testing.T, env *testEnv, envID, name string) {
	t.Helper()
	svc, err := env.app.service()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := svc.Open(ctx, env.app.getCurrent())
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if _, err := u.SetSecret(ctx, domain.ID[domain.Environment](envID), domain.SecretName(name), []byte(seedValue), "", ""); err != nil {
		t.Fatal(err)
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", code)
	}
	if got := codeOf(t, err); got != code {
		t.Fatalf("want %s, got %s", code, got)
	}
}

type fixture struct {
	env           *testEnv
	web, api      ProjectDTO
	dev, prod, ci EnvDTO
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	env := newTestApp(t)
	mustCreate(t, env, "work")
	a := env.app
	var f fixture
	var err error
	f.env = env
	if f.web, err = a.CreateProject("web"); err != nil {
		t.Fatal(err)
	}
	if f.api, err = a.CreateProject("API"); err != nil {
		t.Fatal(err)
	}
	if f.prod, err = a.CreateEnvironment(f.web.ID, "prod"); err != nil {
		t.Fatal(err)
	}
	if f.dev, err = a.CreateEnvironment(f.web.ID, "dev"); err != nil {
		t.Fatal(err)
	}
	if f.ci, err = a.CreateEnvironment(f.api.ID, "ci"); err != nil {
		t.Fatal(err)
	}
	seed(t, env, f.dev.ID, seedName)
	seed(t, env, f.dev.ID, seedName+"_2")
	seed(t, env, f.ci.ID, seedName)
	return f
}

func TestCreateReturnsZeroCounts(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	p, err := env.app.CreateProject("web")
	if err != nil || p.ID == "" || p.Name != "web" || p.EnvCount != 0 || p.SecretCount != 0 {
		t.Fatalf("%+v %v", p, err)
	}
	e, err := env.app.CreateEnvironment(p.ID, "dev")
	if err != nil || e.ID == "" || e.Name != "dev" || e.SecretCount != 0 {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestListCountsAndSorting(t *testing.T) {
	f := newFixture(t)
	ps, err := f.env.app.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	want := []ProjectDTO{
		{ID: f.api.ID, Name: "API", EnvCount: 1, SecretCount: 1},
		{ID: f.web.ID, Name: "web", EnvCount: 2, SecretCount: 2},
	}
	if len(ps) != 2 || ps[0] != want[0] || ps[1] != want[1] {
		t.Fatalf("got %+v", ps)
	}
	es, err := f.env.app.ListEnvironments(f.web.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0] != (EnvDTO{ID: f.dev.ID, Name: "dev", SecretCount: 2}) || es[1] != (EnvDTO{ID: f.prod.ID, Name: "prod"}) {
		t.Fatalf("got %+v", es)
	}
	// case-insensitive ordering for envs too
	if _, err := f.env.app.CreateEnvironment(f.web.ID, "Beta"); err != nil {
		t.Fatal(err)
	}
	es, _ = f.env.app.ListEnvironments(f.web.ID)
	if es[0].Name != "Beta" || es[1].Name != "dev" || es[2].Name != "prod" {
		t.Fatalf("got %+v", es)
	}
}

func TestEmptyLists(t *testing.T) {
	env := newTestApp(t)
	mustCreate(t, env, "work")
	ps, err := env.app.ListProjects()
	if err != nil || ps == nil || len(ps) != 0 {
		t.Fatalf("%#v %v", ps, err)
	}
	p, _ := env.app.CreateProject("x")
	es, err := env.app.ListEnvironments(p.ID)
	if err != nil || es == nil || len(es) != 0 {
		t.Fatalf("%#v %v", es, err)
	}
}

func TestDeleteProjectCascades(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	if err := a.DeleteProject(f.web.ID); err != nil {
		t.Fatal(err)
	}
	ps, _ := a.ListProjects()
	if len(ps) != 1 || ps[0].ID != f.api.ID || ps[0].SecretCount != 1 {
		t.Fatalf("got %+v", ps)
	}
	_, err := a.ListEnvironments(f.web.ID)
	wantCode(t, err, CodeNotFound)
	wantCode(t, a.DeleteEnvironment(f.dev.ID), CodeNotFound)
	_, err = a.CreateEnvironment(f.web.ID, "x")
	wantCode(t, err, CodeNotFound)
	wantCode(t, a.DeleteProject(f.web.ID), CodeNotFound)
}

func TestDeleteEnvUpdatesCounts(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	if err := a.DeleteEnvironment(f.dev.ID); err != nil {
		t.Fatal(err)
	}
	ps, _ := a.ListProjects()
	if ps[1].ID != f.web.ID || ps[1].EnvCount != 1 || ps[1].SecretCount != 0 {
		t.Fatalf("got %+v", ps)
	}
	wantCode(t, a.DeleteEnvironment(f.dev.ID), CodeNotFound)
}

func TestDuplicates(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	_, err := a.CreateProject("web")
	wantCode(t, err, CodeDuplicate)
	_, err = a.CreateEnvironment(f.web.ID, "dev")
	wantCode(t, err, CodeDuplicate)
	if _, err := a.CreateEnvironment(f.api.ID, "dev"); err != nil {
		t.Fatalf("same env name in another project: %v", err)
	}
}

func TestInvalidNames(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	for _, n := range []string{"", "   ", "a\nb", "a\x00b", "a\tb", "\x7f"} {
		_, err := a.CreateProject(n)
		wantCode(t, err, CodeInvalidName)
		_, err = a.CreateEnvironment(f.web.ID, n)
		wantCode(t, err, CodeInvalidName)
	}
}

func TestWrongKindIDsNotFound(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	_, err := a.ListEnvironments(f.dev.ID)
	wantCode(t, err, CodeNotFound)
	_, err = a.CreateEnvironment(f.dev.ID, "x")
	wantCode(t, err, CodeNotFound)
	wantCode(t, a.DeleteProject(f.dev.ID), CodeNotFound)
	wantCode(t, a.DeleteEnvironment(f.web.ID), CodeNotFound)
	wantCode(t, a.DeleteProject("nope"), CodeNotFound)
	// nothing was removed
	if ps, _ := a.ListProjects(); len(ps) != 2 {
		t.Fatalf("got %+v", ps)
	}
}

func TestNoVaultAndLocked(t *testing.T) {
	env := newTestApp(t)
	a := env.app
	check := func(code string) {
		t.Helper()
		_, err := a.ListProjects()
		wantCode(t, err, code)
		_, err = a.CreateProject("x")
		wantCode(t, err, code)
		wantCode(t, a.DeleteProject("x"), code)
		_, err = a.ListEnvironments("x")
		wantCode(t, err, code)
		_, err = a.CreateEnvironment("x", "y")
		wantCode(t, err, code)
		wantCode(t, a.DeleteEnvironment("x"), code)
	}
	check(CodeNoVault)
	mustCreate(t, env, "work")
	if err := a.Lock(); err != nil {
		t.Fatal(err)
	}
	check(CodeLocked)
}

func TestNoSecretsInDTOs(t *testing.T) {
	f := newFixture(t)
	a := f.env.app
	ps, _ := a.ListProjects()
	es, _ := a.ListEnvironments(f.web.ID)
	es2, _ := a.ListEnvironments(f.api.ID)
	all, err := json.Marshal([]any{ps, es, es2, f.web, f.api, f.dev, f.prod, f.ci})
	if err != nil {
		t.Fatal(err)
	}
	s := string(all)
	if strings.Contains(s, seedValue) || strings.Contains(s, seedName) {
		t.Fatalf("leak: %s", s)
	}
	for _, k := range []string{`"id"`, `"name"`, `"envCount"`, `"secretCount"`} {
		if !strings.Contains(s, k) {
			t.Fatalf("missing json key %s in %s", k, s)
		}
	}
}

func TestOneOpenPerCall(t *testing.T) {
	f := newFixture(t)
	a, keys := f.env.app, f.env.keys
	calls := map[string]func(){
		"ListProjects":      func() { a.ListProjects() },
		"CreateProject":     func() { a.CreateProject("p2") },
		"ListEnvironments":  func() { a.ListEnvironments(f.web.ID) },
		"CreateEnvironment": func() { a.CreateEnvironment(f.web.ID, "e2") },
		"DeleteEnvironment": func() { a.DeleteEnvironment(f.prod.ID) },
		"DeleteProject":     func() { a.DeleteProject(f.api.ID) },
	}
	for name, call := range calls {
		before := keys.loads.Load()
		call()
		if got := keys.loads.Load() - before; got != 1 {
			t.Errorf("%s: %d opens", name, got)
		}
	}
}
