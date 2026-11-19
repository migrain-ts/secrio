package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"

	"secrio/internal/domain"
)

const saltSize = 16

// DefaultKDFParams returns the Argon2id cost used for new vaults.
func DefaultKDFParams() domain.KDFParams {
	return domain.KDFParams{Time: 4, MemoryKiB: 256 * 1024, Threads: 4}
}

// RandomSalt returns 16 random bytes.
func (Suite) RandomSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

// Derive computes a 32-byte key from password and salt using Argon2id.
func (Suite) Derive(password, salt []byte, p domain.KDFParams) domain.Key {
	var k domain.Key
	raw := argon2.IDKey(password, salt, p.Time, p.MemoryKiB, p.Threads, 32)
	copy(k[:], raw)
	clear(raw)
	return k
}
