package desktop

import (
	"encoding/json"
	"errors"

	"secrio/internal/domain"
)

// Error codes the frontend switches on.
const (
	CodeLocked          = "locked"
	CodeInvalidPassword = "invalid_password"
	CodeNotFound        = "not_found"
	CodeDuplicate       = "duplicate"
	CodeInvalidName     = "invalid_name"
	CodeInvalidValue    = "invalid_value"
	CodeConflict        = "conflict"
	CodeUnsupported     = "unsupported"
	CodeNoVault         = "no_vault"
	CodeInternal        = "internal"
)

// apiError is what bound methods return; Error() is JSON so the frontend can
// parse it. Messages are fixed texts, never names, paths, values or passwords.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return `{"code":"internal","message":"unexpected error"}`
	}
	return string(b)
}

func newAPIError(code, msg string) error { return &apiError{Code: code, Message: msg} }

// toAPIError maps an error to an *apiError. Unknown errors never leak text.
func toAPIError(err error) error {
	if err == nil {
		return nil
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return ae
	}
	switch {
	case errors.Is(err, domain.ErrLocked):
		return newAPIError(CodeLocked, "vault is locked")
	case errors.Is(err, domain.ErrInvalidPassword):
		return newAPIError(CodeInvalidPassword, "invalid password")
	case errors.Is(err, domain.ErrNotFound):
		return newAPIError(CodeNotFound, "not found")
	case errors.Is(err, domain.ErrDuplicate):
		return newAPIError(CodeDuplicate, "already exists")
	case errors.Is(err, domain.ErrInvalidName):
		return newAPIError(CodeInvalidName, "invalid name")
	case errors.Is(err, domain.ErrInvalidArgument), errors.Is(err, domain.ErrUnsupportedValue):
		return newAPIError(CodeInvalidValue, "invalid value")
	case errors.Is(err, domain.ErrCorrupt), errors.Is(err, domain.ErrUnsupportedFormat):
		return newAPIError(CodeInternal, "vault data is damaged or unsupported")
	}
	return newAPIError(CodeInternal, "unexpected error")
}

// unsupported is returned when an optional dependency (clipboard, dialogs)
// was not provided.
func unsupported() error { return newAPIError(CodeUnsupported, "not supported here") }
