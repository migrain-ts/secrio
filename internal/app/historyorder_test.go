package app

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"secrio/internal/domain"
)

func historyNames(t *testing.T, u *Unlocked, e domain.Environment) []string {
	t.Helper()
	es, err := u.History(context.Background(), e.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, x := range es {
		if x.SecretName != "" { // skip the environment's own events
			out = append(out, string(x.SecretName))
		}
	}
	return out
}

func TestHistoryOrderManySameInstant(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	var want []string
	for i := 1; i <= 7; i++ { // the fake clock never advances
		n := fmt.Sprintf("K%d", i)
		if _, err := u.SetSecret(ctx, e.ID, domain.SecretName(n), []byte("v"), "", ""); err != nil {
			t.Fatal(err)
		}
		want = slices.Insert(want, 0, n) // newest first
	}
	if got := historyNames(t, u, e); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestHistoryOrderWhenClockGoesBackwards(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	_, e := newEnv(t, u, "p", "Dev")
	for _, n := range []string{"K1", "K2", "K3", "K4"} {
		if _, err := u.SetSecret(ctx, e.ID, domain.SecretName(n), []byte("v"), "", ""); err != nil {
			t.Fatal(err)
		}
		h.clock.Advance(-time.Hour)
	}
	want := []string{"K4", "K3", "K2", "K1"}
	if got := historyNames(t, u, e); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
