package app

import (
	"errors"
	"fmt"

	"secrio/internal/domain"
)

const (
	vaultKeyAAD = "vaultkey"

	// formatVersion is the only vault header version this build understands.
	formatVersion = 1

	maxKDFTime      = 64 // caps against pre-auth CPU denial of service
	maxKDFThreads   = 64
	minKDFMemoryKiB = 8
	maxKDFMemoryKiB = 1 << 20 // 1 GiB cap against hostile vault files
)

// sessionAAD binds a session blob to its vault, so a session row and key
// grafted from another vault do not open.
func sessionAAD(vaultID string) []byte { return []byte("session|" + vaultID) }

// zeroKey clears a key held in memory.
func zeroKey(k *domain.Key) { *k = domain.Key{} }

// validKDFParams reports whether p is within sane bounds.
func validKDFParams(p domain.KDFParams) bool {
	return p.Time >= 1 && p.Time <= maxKDFTime &&
		p.Threads >= 1 && p.Threads <= maxKDFThreads &&
		p.MemoryKiB >= minKDFMemoryKiB && p.MemoryKiB <= maxKDFMemoryKiB
}

// newVaultKeys creates vault metadata (fresh ID, salt) and a new vault key
// wrapped under a key derived from password.
func newVaultKeys(c Cipher, kdf KDF, p domain.KDFParams, password []byte) (meta VaultMeta, vaultKey domain.Key, err error) {
	if !validKDFParams(p) {
		return VaultMeta{}, domain.Key{}, fmt.Errorf("invalid kdf params")
	}
	salt, err := kdf.RandomSalt()
	if err != nil {
		return VaultMeta{}, domain.Key{}, fmt.Errorf("salt: %w", err)
	}
	vaultKey, err = c.NewKey()
	if err != nil {
		return VaultMeta{}, domain.Key{}, fmt.Errorf("vault key: %w", err)
	}
	unlock := kdf.Derive(password, salt, p)
	defer zeroKey(&unlock)
	wrapped, err := c.Seal(unlock, vaultKey[:], []byte(vaultKeyAAD))
	if err != nil {
		zeroKey(&vaultKey)
		return VaultMeta{}, domain.Key{}, fmt.Errorf("wrap vault key: %w", err)
	}
	meta = VaultMeta{
		FormatVersion:   formatVersion,
		ID:              string(domain.NewID[domain.Vault]()),
		KDF:             p,
		Salt:            salt,
		WrappedVaultKey: wrapped,
	}
	return meta, vaultKey, nil
}

// unwrapVaultKey derives the unlock key from password and opens the vault key.
// A wrong password yields domain.ErrInvalidPassword.
func unwrapVaultKey(c Cipher, kdf KDF, meta VaultMeta, password []byte) (domain.Key, error) {
	if meta.FormatVersion != formatVersion {
		return domain.Key{}, fmt.Errorf("%w: vault format version %d", domain.ErrUnsupportedFormat, meta.FormatVersion)
	}
	if !validKDFParams(meta.KDF) {
		return domain.Key{}, fmt.Errorf("%w: kdf params out of range", domain.ErrCorrupt)
	}
	unlock := kdf.Derive(password, meta.Salt, meta.KDF)
	defer zeroKey(&unlock)
	pt, err := c.Open(unlock, meta.WrappedVaultKey, []byte(vaultKeyAAD))
	if err != nil {
		if errors.Is(err, domain.ErrAuthFailed) {
			return domain.Key{}, domain.ErrInvalidPassword
		}
		return domain.Key{}, err
	}
	return keyFromBytes(pt)
}

// rewrapVaultKey re-encrypts the same vault key under newPw with a new salt.
// The new wrap uses the existing KDF params.
func rewrapVaultKey(c Cipher, kdf KDF, meta VaultMeta, oldPw, newPw []byte) (VaultMeta, error) {
	vk, err := unwrapVaultKey(c, kdf, meta, oldPw)
	if err != nil {
		return VaultMeta{}, err
	}
	defer zeroKey(&vk)
	salt, err := kdf.RandomSalt()
	if err != nil {
		return VaultMeta{}, fmt.Errorf("salt: %w", err)
	}
	unlock := kdf.Derive(newPw, salt, meta.KDF)
	defer zeroKey(&unlock)
	wrapped, err := c.Seal(unlock, vk[:], []byte(vaultKeyAAD))
	if err != nil {
		return VaultMeta{}, fmt.Errorf("wrap vault key: %w", err)
	}
	meta.Salt = salt
	meta.WrappedVaultKey = wrapped
	return meta, nil
}

// newEnvKey generates an environment key wrapped under the vault key, bound
// to envID.
func newEnvKey(c Cipher, vaultKey domain.Key, envID string) (key domain.Key, wrapped []byte, err error) {
	key, err = c.NewKey()
	if err != nil {
		return domain.Key{}, nil, fmt.Errorf("env key: %w", err)
	}
	wrapped, err = c.Seal(vaultKey, key[:], []byte(envID))
	if err != nil {
		zeroKey(&key)
		return domain.Key{}, nil, fmt.Errorf("wrap env key: %w", err)
	}
	return key, wrapped, nil
}

// unwrapEnvKey opens a wrapped environment key. Authentication failure with
// the correct vault key means stored data is corrupt or swapped.
func unwrapEnvKey(c Cipher, vaultKey domain.Key, envID string, wrapped []byte) (domain.Key, error) {
	pt, err := c.Open(vaultKey, wrapped, []byte(envID))
	if err != nil {
		if errors.Is(err, domain.ErrAuthFailed) {
			return domain.Key{}, fmt.Errorf("%w: env key does not authenticate", domain.ErrCorrupt)
		}
		return domain.Key{}, err
	}
	return keyFromBytes(pt)
}

// keyFromBytes copies pt into a Key and zeroes pt.
func keyFromBytes(pt []byte) (domain.Key, error) {
	defer clear(pt)
	var k domain.Key
	if len(pt) != len(k) {
		return domain.Key{}, fmt.Errorf("%w: key length %d", domain.ErrCorrupt, len(pt))
	}
	copy(k[:], pt)
	return k, nil
}
