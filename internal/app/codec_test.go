package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"secrio/internal/domain"
)

func TestSealOpenRoundTripGeneric(t *testing.T) {
	c := newTestCipher()
	k, _ := c.NewKey()

	s := domain.Secret{ID: "s1", EnvID: "e1", Name: "API_KEY", Value: []byte("v\x00\xff"),
		Type: domain.TypeAPIKey, CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC()}
	blob, err := seal(c, k, KindSecret, "s1", s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := open[domain.Secret](c, k, KindSecret, "s1", blob)
	if err != nil || !reflect.DeepEqual(got, s) {
		t.Fatalf("secret: %+v, %v", got, err)
	}

	p := domain.Project{ID: "p1", Name: "n", Description: "d", CreatedAt: time.Unix(3, 0).UTC()}
	blob, err = seal(c, k, KindProject, "p1", p)
	if err != nil {
		t.Fatal(err)
	}
	gp, err := open[domain.Project](c, k, KindProject, "p1", blob)
	if err != nil || gp != p {
		t.Fatalf("project: %+v, %v", gp, err)
	}
}

func TestOpenWrongKindFails(t *testing.T) {
	c := newTestCipher()
	k, _ := c.NewKey()
	blob, _ := seal(c, k, KindProject, "x", domain.Project{Name: "n"})
	if _, err := open[domain.Project](c, k, KindEnvironment, "x", blob); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
}

func TestOpenWrongIDFails(t *testing.T) {
	c := newTestCipher()
	k, _ := c.NewKey()
	blob, _ := seal(c, k, KindProject, "x", domain.Project{Name: "n"})
	if _, err := open[domain.Project](c, k, KindProject, "y", blob); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("want ErrAuthFailed, got %v", err)
	}
}
