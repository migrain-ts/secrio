// Package crypto implements sealing (XChaCha20-Poly1305) and key derivation
// (Argon2id) for secrio.
package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"

	"secrio/internal/domain"
)

const versionByte byte = 0x01

// Suite provides all cryptographic primitives. It holds no state.
type Suite struct{}

// NewKey returns a fresh random 32-byte key.
func (Suite) NewKey() (domain.Key, error) {
	var k domain.Key
	if _, err := rand.Read(k[:]); err != nil {
		return domain.Key{}, fmt.Errorf("generate key: %w", err)
	}
	return k, nil
}

// Seal encrypts plaintext, returning 0x01 | nonce(24) | ciphertext.
func (Suite) Seal(k domain.Key, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k[:])
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	out := make([]byte, 1+aead.NonceSize(), 1+aead.NonceSize()+len(plaintext)+aead.Overhead())
	out[0] = versionByte
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	nonce := out[1:]
	return aead.Seal(out, nonce, plaintext, aad), nil
}

// Open authenticates and decrypts a blob produced by Seal.
func (Suite) Open(k domain.Key, sealed, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k[:])
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	ns := aead.NonceSize()
	if len(sealed) < 1+ns+aead.Overhead() {
		return nil, domain.ErrCorrupt
	}
	if sealed[0] != versionByte {
		return nil, domain.ErrUnsupportedFormat
	}
	pt, err := aead.Open(nil, sealed[1:1+ns], sealed[1+ns:], aad)
	if err != nil {
		return nil, domain.ErrAuthFailed
	}
	return pt, nil
}

// Zero overwrites b with zeros.
func Zero(b []byte) {
	clear(b)
}
