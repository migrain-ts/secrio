package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// ID is a typed random identifier (16 random bytes, hex encoded).
type ID[T any] string

// NewID returns a new random ID.
func NewID[T any]() ID[T] {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("domain: crypto/rand failed: " + err.Error())
	}
	return ID[T](hex.EncodeToString(b[:]))
}
