package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"secrio/internal/domain"
)

// newEnv creates a project and one environment.
func newEnv(t *testing.T, u *Unlocked, project, env string) (domain.Project, domain.Environment) {
	t.Helper()
	ctx := context.Background()
	p, err := u.CreateProject(ctx, project, "")
	if err != nil {
		t.Fatal(err)
	}
	e, err := u.CreateEnvironment(ctx, p.ID, env)
	if err != nil {
		t.Fatal(err)
	}
	return p, e
}

func TestSecretValueRoundTrip(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, id := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	big := make([]byte, 1<<20)
	rand.Read(big)
	tests := []struct {
		name  string
		value []byte
	}{
		{"empty", []byte("")},
		{"newline", []byte("a\nb")},
		{"equals", []byte("k=v")},
		{"nul", []byte("\x00x")},
		{"1MiB", big},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := domain.SecretName("S" + string(rune('A'+i)))
			if _, err := u.SetSecret(ctx, e.ID, name, tt.value, domain.TypeText, ""); err != nil {
				t.Fatal(err)
			}
			u2, err := h.svc.Open(ctx, id) // fresh handle reads from the store
			if err != nil {
				t.Fatal(err)
			}
			defer u2.Close()
			got, err := u2.GetSecret(ctx, e.ID, name)
			if err != nil || !bytes.Equal(got.Value, tt.value) || len(got.Value) != len(tt.value) {
				t.Fatalf("round trip mismatch (len %d vs %d): %v", len(got.Value), len(tt.value), err)
			}
		})
	}
}

func TestSetSecretDuplicateNameUpserts(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	first, err := u.SetSecret(ctx, e.ID, "DB_URL", []byte("v1"), domain.TypeURL, "d1")
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	second, err := u.SetSecret(ctx, e.ID, "DB_URL", []byte("v2"), domain.TypeURL, "d2")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) || !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("upsert metadata: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}
	all, err := u.ListSecrets(ctx, e.ID)
	if err != nil || len(all) != 1 || string(all[0].Value) != "v2" || all[0].Description != "d2" {
		t.Fatalf("list after upsert: n=%d %v", len(all), err)
	}
}

func TestCreateSecretCaseSensitive(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	u.SetSecret(ctx, e.ID, "a", []byte("lower"), "", "")
	u.SetSecret(ctx, e.ID, "A", []byte("upper"), "", "")
	all, err := u.ListSecrets(ctx, e.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("n=%d %v", len(all), err)
	}
	a, _ := u.GetSecret(ctx, e.ID, "a")
	A, _ := u.GetSecret(ctx, e.ID, "A")
	if string(a.Value) != "lower" || string(A.Value) != "upper" {
		t.Fatal("case-sensitive names collided")
	}
}

func TestSecretTypeAndNameValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	s, err := u.SetSecret(ctx, e.ID, "X", []byte("v"), "", "")
	if err != nil || s.Type != domain.TypeText {
		t.Fatalf("default type = %q, %v", s.Type, err)
	}
	if _, err := u.SetSecret(ctx, e.ID, "X", []byte("v"), "bogus", ""); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("unknown type: %v", err)
	}
	if _, err := u.SetSecret(ctx, e.ID, "1BAD", []byte("v"), "", ""); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := u.SetSecret(ctx, "nope", "X", []byte("v"), "", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown env: %v", err)
	}
}

func TestDeleteSecret(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	u.SetSecret(ctx, e.ID, "X", []byte("v"), "", "")
	if err := u.DeleteSecret(ctx, e.ID, "X"); err != nil {
		t.Fatal(err)
	}
	if _, err := u.GetSecret(ctx, e.ID, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := u.DeleteSecret(ctx, e.ID, "X"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestDeleteProjectCascades(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e1 := newEnv(t, u, "doomed", "Development")
	e2, _ := u.CreateEnvironment(ctx, p.ID, "Production")
	u.SetSecret(ctx, e1.ID, "A", []byte("1"), "", "")
	u.SetSecret(ctx, e2.ID, "B", []byte("2"), "", "")
	keep, keepEnv := newEnv(t, u, "keep", "Development")
	u.SetSecret(ctx, keepEnv.ID, "C", []byte("3"), "", "")

	if err := u.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	v := h.vault()
	for _, k := range []Kind{KindProject, KindEnvironment, KindEnvKey, KindSecret} {
		if n := v.countRecords(k); n != 1 {
			t.Errorf("%s records left = %d, want 1 (the kept project's)", k, n)
		}
	}
	ps, _ := u.ListProjects(ctx)
	if len(ps) != 1 || ps[0].ID != keep.ID {
		t.Fatalf("projects = %v", ps)
	}
	if _, err := u.GetSecret(ctx, e1.ID, "A"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("secret in deleted env: %v", err)
	}
	if c, err := u.GetSecret(ctx, keepEnv.ID, "C"); err != nil || string(c.Value) != "3" {
		t.Fatalf("kept secret: %v", err)
	}
	if err := u.DeleteProject(ctx, p.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestEnvironments(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, e := newEnv(t, u, "p", "Development")
	if _, err := u.CreateEnvironment(ctx, p.ID, "Development"); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("dup env: %v", err)
	}
	if _, err := u.CreateEnvironment(ctx, "nope", "Development"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown project: %v", err)
	}
	if _, err := u.CreateEnvironment(ctx, p.ID, " "); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("blank: %v", err)
	}
	u.SetSecret(ctx, e.ID, "X", []byte("v"), "", "")
	if err := u.DeleteEnvironment(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	envs, err := u.ListEnvironments(ctx, p.ID)
	if err != nil || len(envs) != 0 || h.vault().countRecords(KindSecret) != 0 || h.vault().countRecords(KindEnvKey) != 0 {
		t.Fatalf("after delete: %v %v", envs, err)
	}
	if err := u.DeleteEnvironment(ctx, e.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestRenameProject(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, _ := u.CreateProject(ctx, "old", "desc")
	u.CreateProject(ctx, "other", "")
	if err := u.RenameProject(ctx, p.ID, "new"); err != nil {
		t.Fatal(err)
	}
	ps, _ := u.ListProjects(ctx)
	if len(ps) != 2 || ps[0].Name != "new" || ps[0].Description != "desc" || ps[0].ID != p.ID {
		t.Fatalf("projects = %+v", ps)
	}
	if err := u.RenameProject(ctx, p.ID, "other"); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("dup: %v", err)
	}
	if err := u.RenameProject(ctx, p.ID, ""); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("blank: %v", err)
	}
	if err := u.RenameProject(ctx, "nope", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := u.RenameProject(ctx, p.ID, "new"); err != nil { // same name is fine
		t.Fatal(err)
	}
}

func TestDuplicateProject(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	src, e := newEnv(t, u, "src", "Development")
	u.SetSecret(ctx, e.ID, "A", []byte("1"), domain.TypeToken, "d")
	u.SetSecret(ctx, e.ID, "B", []byte("2"), "", "")

	cp, err := u.DuplicateProject(ctx, src.ID, "copy")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == src.ID || cp.Name != "copy" {
		t.Fatalf("copy = %+v", cp)
	}
	envs, _ := u.ListEnvironments(ctx, cp.ID)
	if len(envs) != 1 || envs[0].Name != "Development" || envs[0].ID == e.ID || envs[0].ProjectID != cp.ID {
		t.Fatalf("envs = %+v", envs)
	}
	srcKey, _ := u.envKey(ctx, u.store, e.ID)
	cpKey, _ := u.envKey(ctx, u.store, envs[0].ID)
	if srcKey == cpKey {
		t.Fatal("copied env shares the source env key")
	}
	if _, err := u.DuplicateProject(ctx, src.ID, "copy"); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("dup name: %v", err)
	}
	if _, err := u.DuplicateProject(ctx, "nope", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := u.DeleteProject(ctx, src.ID); err != nil {
		t.Fatal(err)
	}
	secrets, err := u.ListSecrets(ctx, envs[0].ID)
	if err != nil || len(secrets) != 2 {
		t.Fatalf("copied secrets: n=%d %v", len(secrets), err)
	}
	a := secrets[0]
	if a.Name != "A" || string(a.Value) != "1" || a.Type != domain.TypeToken || a.Description != "d" || a.EnvID != envs[0].ID {
		t.Fatal("copied secret mismatch")
	}
}

func TestEnvsHaveDistinctKeys(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, a := newEnv(t, u, "p", "A")
	b, _ := u.CreateEnvironment(ctx, p.ID, "B")
	s, err := u.SetSecret(ctx, a.ID, "X", []byte("only-in-a"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	keyA, _ := u.envKey(ctx, u.store, a.ID)
	keyB, err := u.envKey(ctx, u.store, b.ID)
	if err != nil || keyA == keyB {
		t.Fatalf("keys not distinct: %v", err)
	}
	rec, _ := u.store.Get(ctx, KindSecret, string(s.ID))
	if _, err := open[domain.Secret](u.c, keyB, KindSecret, rec.ID, rec.Blob); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("env B key opened env A secret: %v", err)
	}
	if _, err := open[domain.Secret](u.c, u.key, KindSecret, rec.ID, rec.Blob); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("vault key opened a secret: %v", err)
	}
}

func TestCorruptSecretBlobIsCorrupt(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Development")
	s, _ := u.SetSecret(ctx, e.ID, "X", []byte("v"), "", "")
	k := recKey{KindSecret, string(s.ID)}
	r := h.vault().records[k]
	r.Blob[len(r.Blob)-1] ^= 1 // authentication failure, not truncation
	h.vault().records[k] = r
	_, err := u.GetSecret(ctx, e.ID, "X")
	if !errors.Is(err, domain.ErrCorrupt) || errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}

func TestErrorsNeverContainValue(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, id := h.unlocked(t)
	const value = "s3cr3t-VALUE-xyz"
	p, e := newEnv(t, u, "Proj_Name_123", "Env_Name_456")
	s, _ := u.SetSecret(ctx, e.ID, "SECRET_NAME_789", []byte(value), "", "")

	var errs []error
	_, err := u.GetSecret(ctx, e.ID, "MISSING_NAME_000")
	errs = append(errs, err) // ErrNotFound
	_, err = u.CreateProject(ctx, "Proj_Name_123", "")
	errs = append(errs, err) // ErrDuplicate
	_, err = u.CreateEnvironment(ctx, p.ID, "Env_Name_456")
	errs = append(errs, err) // ErrDuplicate
	errs = append(errs, h.svc.Unlock(ctx, id, []byte("WRONG-pw-321"), time.Hour, 0))
	errs = append(errs, h.svc.ChangePassword(ctx, id, []byte("WRONG-pw-321"), []byte("NEW-pw-654")))
	k := recKey{KindSecret, string(s.ID)}
	r := h.vault().records[k]
	r.Blob[len(r.Blob)-1] ^= 1
	h.vault().records[k] = r
	_, err = u.GetSecret(ctx, e.ID, "SECRET_NAME_789")
	errs = append(errs, err) // ErrCorrupt (auth failure)
	r.Blob = r.Blob[:3]
	h.vault().records[k] = r
	_, err = u.ListSecrets(ctx, e.ID)
	errs = append(errs, err) // ErrCorrupt (truncated)

	want := []error{domain.ErrNotFound, domain.ErrDuplicate, domain.ErrDuplicate, domain.ErrInvalidPassword,
		domain.ErrInvalidPassword, domain.ErrCorrupt, domain.ErrCorrupt}
	forbidden := []string{value, testPw, "WRONG-pw-321", "NEW-pw-654", "SECRET_NAME_789", "MISSING_NAME_000",
		"Proj_Name_123", "Env_Name_456"}
	for i, err := range errs {
		if !errors.Is(err, want[i]) {
			t.Fatalf("case %d: want %v, got %v", i, want[i], err)
		}
		for _, f := range forbidden {
			if strings.Contains(err.Error(), f) {
				t.Errorf("case %d: error %q leaks %q", i, err, f)
			}
		}
	}
}
