package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"secrio/internal/domain"
)

func envWith(t *testing.T, kv map[string]string) (*Unlocked, domain.Environment) {
	t.Helper()
	u, _ := newHarness(t).unlocked(t)
	_, e := newEnv(t, u, "p", "dev")
	for k, v := range kv {
		if _, err := u.SetSecret(context.Background(), e.ID, domain.SecretName(k), []byte(v), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	return u, e
}

func TestProcessEnvOverridesOnlySameName(t *testing.T) {
	u, e := envWith(t, map[string]string{"DB_URL": "new", "ZED": "z", "ALPHA": "a"})
	base := []string{"HOME=/h", "DB_URL=old", "OTHER=1", "DB_URL=dup", "BROKEN", "db_url=lower"}
	got, err := u.ProcessEnv(context.Background(), e.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	// db_url is a different name on unix; on windows it is dropped as a duplicate.
	want := []string{"HOME=/h", "DB_URL=new", "OTHER=1", "BROKEN", "db_url=lower", "ALPHA=a", "ZED=z"}
	if envFold {
		want = []string{"HOME=/h", "DB_URL=new", "OTHER=1", "BROKEN", "ALPHA=a", "ZED=z"}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestProcessEnvKeepsPath(t *testing.T) {
	u, e := envWith(t, map[string]string{"TOKEN": "t"})
	got, err := u.ProcessEnv(context.Background(), e.ID, []string{"PATH=/usr/bin:/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"PATH=/usr/bin:/bin", "TOKEN=t"}) {
		t.Fatalf("got %q", got)
	}
}

func TestProcessEnvValueWithEqualsAndNewline(t *testing.T) {
	u, e := envWith(t, map[string]string{"K": "a=b\nc=d="})
	got, err := u.ProcessEnv(context.Background(), e.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"K=a=b\nc=d="}) {
		t.Fatalf("got %q", got)
	}
}

func TestProcessEnvEmptyEnvCopiesBase(t *testing.T) {
	u, e := envWith(t, nil)
	base := []string{"A=1", "B"}
	got, err := u.ProcessEnv(context.Background(), e.ID, base)
	if err != nil || !reflect.DeepEqual(got, base) {
		t.Fatalf("got %q %v", got, err)
	}
	got[0] = "A=changed"
	if base[0] != "A=1" {
		t.Fatal("result aliases base")
	}
}

func TestProcessEnvUnknownEnv(t *testing.T) {
	u, _ := envWith(t, nil)
	_, err := u.ProcessEnv(context.Background(), domain.NewID[domain.Environment](), nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestProcessEnvNULValue(t *testing.T) {
	u, e := envWith(t, map[string]string{"SECRET_NAME": "xq\x00zv"})
	_, err := u.ProcessEnv(context.Background(), e.ID, nil)
	if !errors.Is(err, domain.ErrUnsupportedValue) {
		t.Fatalf("got %v", err)
	}
	for _, leak := range []string{"SECRET_NAME", "xq", "zv"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaks %q: %v", leak, err)
		}
	}
}

func TestEnvNameEq(t *testing.T) {
	if envNameEq("Path", "PATH", false) || !envNameEq("Path", "PATH", true) || !envNameEq("A", "A", false) {
		t.Fatal("envNameEq")
	}
	if envNameEq("A", "B", true) {
		t.Fatal("different names equal")
	}
}

func TestMergeEnvFolded(t *testing.T) {
	secrets := []domain.Secret{{Name: "PATH", Value: []byte("new")}}
	got := mergeEnv([]string{"Path=x", "PATH=y", "=C:=C:\\"}, secrets, true)
	if !reflect.DeepEqual(got, []string{"PATH=new", "=C:=C:\\"}) {
		t.Fatalf("got %q", got)
	}
	got = mergeEnv([]string{"Path=x"}, secrets, false)
	if !reflect.DeepEqual(got, []string{"Path=x", "PATH=new"}) {
		t.Fatalf("got %q", got)
	}
}
