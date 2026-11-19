package domain

import "errors"

var (
	ErrInvalidName       = errors.New("invalid name")
	ErrLocked            = errors.New("vault is locked")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrNotFound          = errors.New("not found")
	ErrDuplicate         = errors.New("duplicate")
	ErrCorrupt           = errors.New("corrupt data")
	ErrUnsupportedFormat = errors.New("unsupported format")
	ErrAuthFailed        = errors.New("authentication failed")
	ErrUnsupportedValue  = errors.New("unsupported secret value")
)
