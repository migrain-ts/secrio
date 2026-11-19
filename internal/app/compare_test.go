package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"secrio/internal/domain"
)

func setupTwoEnvs(t *testing.T) (*Unlocked, domain.Environment, domain.Environment) {
	t.Helper()
	ctx := context.Background()
	u, _ := newHarness(t).unlocked(t)
	p, dev := newEnv(t, u, "p", "dev")
	staging, err := u.CreateEnvironment(ctx, p.ID, "staging")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"DB_URL", "REDIS_URL"} {
		if _, err := u.SetSecret(ctx, dev.ID, domain.SecretName(n), []byte("v-"+n), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := u.SetSecret(ctx, staging.ID, "DB_URL", []byte("s"), "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := u.SetSecret(ctx, staging.ID, "ONLY_STAGING", []byte("s"), "", ""); err != nil {
		t.Fatal(err)
	}
	return u, dev, staging
}

func TestCompareEnvsPresence(t *testing.T) {
	u, dev, staging := setupTwoEnvs(t)
	got, err := u.CompareEnvs(context.Background(), dev.ID, staging.ID, dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := Presence{
		"DB_URL":       {dev.ID: true, staging.ID: true},
		"REDIS_URL":    {dev.ID: true, staging.ID: false},
		"ONLY_STAGING": {dev.ID: false, staging.ID: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if v, ok := got["REDIS_URL"][staging.ID]; !ok || v {
		t.Fatal("absence must be an explicit false entry")
	}
}

func TestCompareHasNoValues(t *testing.T) {
	typ := reflect.TypeOf(Presence{})
	if typ.Kind() != reflect.Map || typ.Elem().Kind() != reflect.Map || typ.Elem().Elem().Kind() != reflect.Bool {
		t.Fatalf("Presence must be map[name]map[env]bool, got %v", typ)
	}
}

func TestCompareEnvsEdgeCases(t *testing.T) {
	ctx := context.Background()
	u, dev, _ := setupTwoEnvs(t)
	got, err := u.CompareEnvs(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("no ids: %v %v", got, err)
	}
	_, err = u.CompareEnvs(ctx, dev.ID, domain.NewID[domain.Environment]())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown env: %v", err)
	}
}
