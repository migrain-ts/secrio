package app

import (
	"encoding/json"
	"fmt"

	"secrio/internal/domain"
)

// aad binds a blob to its record so it cannot be swapped between records.
func aad(kind Kind, id string) []byte { return []byte(string(kind) + "|" + id) }

// seal JSON-encodes v and encrypts it bound to (kind, id).
func seal[T any](c Cipher, k domain.Key, kind Kind, id string, v T) ([]byte, error) {
	pt, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", kind, err)
	}
	return c.Seal(k, pt, aad(kind, id))
}

// open decrypts blob bound to (kind, id) and JSON-decodes it. Cipher errors
// (ErrAuthFailed, ErrCorrupt) are returned unchanged.
func open[T any](c Cipher, k domain.Key, kind Kind, id string, blob []byte) (T, error) {
	var v T
	pt, err := c.Open(k, blob, aad(kind, id))
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(pt, &v); err != nil {
		return v, fmt.Errorf("%w: decode %s: %v", domain.ErrCorrupt, kind, err)
	}
	return v, nil
}
