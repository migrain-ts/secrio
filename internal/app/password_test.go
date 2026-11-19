package app

import (
	"context"
	"errors"
	"testing"

	"secrio/internal/domain"
)

func TestCreateVaultRejectsEmptyPassword(t *testing.T) {
	h := newHarness(t)
	for _, pw := range [][]byte{nil, {}} {
		if _, err := h.svc.CreateVault(context.Background(), "x", testPath, pw); !errors.Is(err, domain.ErrInvalidPassword) {
			t.Fatalf("got %v", err)
		}
	}
	if len(h.opener.files) != 0 || len(h.reg.entries) != 0 {
		t.Fatal("vault created despite empty password")
	}
}

func TestChangePasswordRejectsEmptyNewPassword(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	if err := h.svc.ChangePassword(ctx, id, []byte(testPw), nil); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("got %v", err)
	}
	u, err := h.svc.OpenWithPassword(ctx, id, []byte(testPw))
	if err != nil {
		t.Fatalf("old password broken: %v", err)
	}
	u.Close()
}
