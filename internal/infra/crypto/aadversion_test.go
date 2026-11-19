package crypto

import (
	"errors"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"

	"secrio/internal/domain"
)

// A blob whose AEAD aad lacks the version prefix must not open: the version
// byte is part of what Seal authenticates.
func TestOpenRequiresVersionInAAD(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	aad := []byte("ctx")
	aead, err := chacha20poly1305.NewX(k[:])
	if err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, 1+aead.NonceSize())
	blob[0] = versionByte                                  // nonce stays zero; fine for a one-off test
	blob = aead.Seal(blob, blob[1:], []byte("hello"), aad) // aad without version
	if _, err := s.Open(k, blob, aad); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
	// and the same construction with the version prefixed opens
	blob2 := make([]byte, 1+aead.NonceSize())
	blob2[0] = versionByte
	blob2 = aead.Seal(blob2, blob2[1:], []byte("hello"), append([]byte{versionByte}, aad...))
	if got, err := s.Open(k, blob2, aad); err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
}
