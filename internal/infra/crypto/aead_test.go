package crypto

import (
	"bytes"
	"errors"
	"testing"

	"secrio/internal/domain"
)

func TestSealOpenRoundTrip(t *testing.T) {
	s := Suite{}
	k, err := s.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, 1 << 20} {
		pt := bytes.Repeat([]byte{0xAB}, n)
		sealed, err := s.Seal(k, pt, []byte("aad"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Open(k, sealed, []byte("aad"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("roundtrip mismatch for len %d", n)
		}
	}
}

func TestSealNonceUnique(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	a, _ := s.Seal(k, []byte("x"), nil)
	b, _ := s.Seal(k, []byte("x"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("two seals identical")
	}
}

func TestOpenTamperedCiphertext(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	sealed, _ := s.Seal(k, []byte("hello"), nil)
	for i := 1; i < len(sealed); i++ {
		c := bytes.Clone(sealed)
		c[i] ^= 0x01
		if _, err := s.Open(k, c, nil); !errors.Is(err, domain.ErrAuthFailed) {
			t.Fatalf("byte %d: got %v", i, err)
		}
	}
	c := bytes.Clone(sealed)
	c[0] ^= 0x01
	if _, err := s.Open(k, c, nil); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("version byte: got %v", err)
	}
}

func TestOpenWrongKey(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	k2, _ := s.NewKey()
	sealed, _ := s.Seal(k, []byte("hello"), nil)
	if _, err := s.Open(k2, sealed, nil); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenWrongAAD(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	sealed, _ := s.Seal(k, []byte("hello"), []byte("a"))
	if _, err := s.Open(k, sealed, []byte("b")); !errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenUnknownVersion(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	sealed, _ := s.Seal(k, []byte("hello"), nil)
	sealed[0] = 0x02
	if _, err := s.Open(k, sealed, nil); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenTruncated(t *testing.T) {
	s := Suite{}
	k, _ := s.NewKey()
	for _, n := range []int{0, 1, 24, 40} {
		if _, err := s.Open(k, make([]byte, n), nil); !errors.Is(err, domain.ErrCorrupt) {
			t.Fatalf("len %d: got %v", n, err)
		}
	}
}

func TestZero(t *testing.T) {
	b := []byte{1, 2, 3}
	Zero(b)
	if !bytes.Equal(b, []byte{0, 0, 0}) {
		t.Fatal("not zeroed")
	}
}
