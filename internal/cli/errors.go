package cli

import (
	"errors"
	"fmt"

	"secrio/internal/domain"
)

// Exit codes (outside `run`).
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitLocked  = 3
)

// ExitError carries an explicit process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// UsageErr is an error that exits with code 2.
func UsageErr(format string, a ...any) error {
	return &ExitError{Code: ExitUsage, Err: fmt.Errorf(format, a...)}
}

// LockedErr marks err as "vault locked / invalid password" (code 3).
func LockedErr(err error) error { return &ExitError{Code: ExitLocked, Err: err} }

// exitCode maps an error chain to a process exit code.
func exitCode(err error) int {
	var ee *ExitError
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &ee):
		return ee.Code
	case errors.Is(err, domain.ErrLocked), errors.Is(err, domain.ErrInvalidPassword):
		return ExitLocked
	case errors.Is(err, domain.ErrInvalidArgument):
		return ExitUsage
	default:
		return ExitFailure
	}
}
