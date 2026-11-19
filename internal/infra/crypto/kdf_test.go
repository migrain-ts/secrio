package crypto

import (
	"testing"

	"secrio/internal/domain"
)

var cheap = domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 1}

func TestDeriveDeterministic(t *testing.T) {
	s := Suite{}
	salt1 := []byte("0123456789abcdef")
	salt2 := []byte("fedcba9876543210")
	a := s.Derive([]byte("pw"), salt1, cheap)
	b := s.Derive([]byte("pw"), salt1, cheap)
	c := s.Derive([]byte("pw"), salt2, cheap)
	if a != b {
		t.Fatal("not deterministic")
	}
	if a == c {
		t.Fatal("salt ignored")
	}
}

func TestRandomSalt(t *testing.T) {
	salt, err := Suite{}.RandomSalt()
	if err != nil || len(salt) != 16 {
		t.Fatalf("len %d err %v", len(salt), err)
	}
}

func BenchmarkDerive(b *testing.B) {
	s := Suite{}
	salt := make([]byte, 16)
	p := DefaultKDFParams()
	for i := 0; i < b.N; i++ {
		s.Derive([]byte("password"), salt, p)
	}
}
