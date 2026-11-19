package app

import (
	"context"
	"errors"
	"testing"

	"secrio/internal/domain"
)

func TestNamesRejectControlCharacters(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	u, _ := h.unlocked(t)
	p, err := u.CreateProject(ctx, "ok", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"a\nb", "x\x1b[31m", "tab\there", "nul\x00", "\x7f", "a\u0085b"} {
		if _, err := u.CreateProject(ctx, bad, ""); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("project %q: %v", bad, err)
		}
		if _, err := u.CreateEnvironment(ctx, p.ID, bad); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("environment %q: %v", bad, err)
		}
		if err := u.RenameProject(ctx, p.ID, bad); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("rename project %q: %v", bad, err)
		}
		if _, err := h.svc.CreateVault(ctx, bad, "v-bad.db", []byte(testPw)); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("vault %q: %v", bad, err)
		}
	}
	for _, good := range []string{"My App", "dévelopment", "プロジェクト", "a-b_c.d"} {
		if _, err := u.CreateProject(ctx, good, ""); err != nil {
			t.Errorf("project %q: %v", good, err)
		}
		if _, err := u.CreateEnvironment(ctx, p.ID, good); err != nil {
			t.Errorf("environment %q: %v", good, err)
		}
	}
}
