package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"secrio/internal/domain"
)

func TestAPIErrorJSONShape(t *testing.T) {
	err := newAPIError(CodeLocked, "vault is locked")
	if got, want := err.Error(), `{"code":"locked","message":"vault is locked"}`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	var m map[string]string
	if json.Unmarshal([]byte(err.Error()), &m) != nil || len(m) != 2 {
		t.Fatalf("bad json %s", err.Error())
	}
}

func TestToAPIErrorMapping(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{domain.ErrLocked, CodeLocked},
		{domain.ErrInvalidPassword, CodeInvalidPassword},
		{domain.ErrNotFound, CodeNotFound},
		{domain.ErrDuplicate, CodeDuplicate},
		{domain.ErrInvalidName, CodeInvalidName},
		{domain.ErrInvalidArgument, CodeInvalidValue},
		{domain.ErrUnsupportedValue, CodeInvalidValue},
		{domain.ErrCorrupt, CodeInternal},
		{domain.ErrUnsupportedFormat, CodeInternal},
		{errors.New("boom"), CodeInternal},
		{fmt.Errorf("wrapped /secret/path: %w", domain.ErrNotFound), CodeNotFound},
	}
	for _, tc := range tests {
		got := toAPIError(tc.err)
		if c := codeOf(t, got); c != tc.code {
			t.Errorf("%v: code %s want %s", tc.err, c, tc.code)
		}
		if strings.Contains(got.Error(), "/secret/path") {
			t.Errorf("leaked wrapped text: %s", got)
		}
	}
	if toAPIError(nil) != nil {
		t.Fatal("nil must map to nil")
	}
	pre := newAPIError(CodeConflict, "x")
	if toAPIError(pre) != pre {
		t.Fatal("apiError must pass through")
	}
}

func TestUnknownErrorsDoNotLeak(t *testing.T) {
	got := toAPIError(errors.New("password hunter2 at /home/me/vault.db"))
	if strings.Contains(got.Error(), "hunter2") || strings.Contains(got.Error(), "/home") {
		t.Fatalf("leaked: %s", got)
	}
	if !strings.Contains(got.Error(), "unexpected error") {
		t.Fatalf("want fixed message: %s", got)
	}
	d := toAPIError(domain.ErrCorrupt)
	if !strings.Contains(d.Error(), "vault data is damaged or unsupported") {
		t.Fatalf("got %s", d)
	}
}
