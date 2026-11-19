package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"

	"secrio/internal/domain"
)

// testKDF is a cheap deterministic KDF: SHA-256(salt || password).
type testKDF struct{}

func newTestKDF() KDF { return testKDF{} }

func (testKDF) RandomSalt() ([]byte, error) {
	s := make([]byte, 16)
	_, err := rand.Read(s)
	return s, err
}

func (testKDF) Derive(password, salt []byte, _ domain.KDFParams) domain.Key {
	return domain.Key(sha256.Sum256(append(append([]byte{}, salt...), password...)))
}

// testCipher is a stdlib AES-GCM Cipher honouring the port contract:
// truncated input -> ErrCorrupt, authentication failure -> ErrAuthFailed.
type testCipher struct{}

func newTestCipher() Cipher { return testCipher{} }

func gcm(k domain.Key) cipher.AEAD {
	b, err := aes.NewCipher(k[:])
	if err != nil {
		panic(err)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		panic(err)
	}
	return g
}

func (testCipher) NewKey() (domain.Key, error) {
	var k domain.Key
	_, err := rand.Read(k[:])
	return k, err
}

func (testCipher) Seal(k domain.Key, plaintext, aad []byte) ([]byte, error) {
	g := gcm(k)
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plaintext, aad), nil
}

func (testCipher) Open(k domain.Key, sealed, aad []byte) ([]byte, error) {
	g := gcm(k)
	if len(sealed) < g.NonceSize()+g.Overhead() {
		return nil, domain.ErrCorrupt
	}
	pt, err := g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], aad)
	if err != nil {
		return nil, domain.ErrAuthFailed
	}
	return pt, nil
}
