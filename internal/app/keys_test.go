package app

import (
	"errors"
	"testing"

	"secrio/internal/domain"
)

var testParams = domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 1}

func TestUnwrapWrongPassword(t *testing.T) {
	c, kdf := newTestCipher(), newTestKDF()
	meta, _, err := newVaultKeys(c, kdf, testParams, []byte("right"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unwrapVaultKey(c, kdf, meta, []byte("wrong")); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("got %v", err)
	}
}

func TestNewVaultKeysMeta(t *testing.T) {
	c, kdf := newTestCipher(), newTestKDF()
	meta, vk, err := newVaultKeys(c, kdf, testParams, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.ID) != 32 || meta.FormatVersion != 1 || meta.KDF != testParams || len(meta.Salt) == 0 {
		t.Fatalf("bad meta %+v", meta)
	}
	got, err := unwrapVaultKey(c, kdf, meta, []byte("pw"))
	if err != nil || got != vk {
		t.Fatalf("roundtrip: %v", err)
	}
	if _, _, err := newVaultKeys(c, kdf, domain.KDFParams{}, []byte("pw")); err == nil {
		t.Fatal("invalid params accepted")
	}
}

func TestRewrapKeepsVaultKey(t *testing.T) {
	c, kdf := newTestCipher(), newTestKDF()
	meta, vk, _ := newVaultKeys(c, kdf, testParams, []byte("old"))
	meta.Name = "n"
	nm, err := rewrapVaultKey(c, kdf, meta, []byte("old"), []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if nm.ID != meta.ID || nm.Name != "n" || nm.FormatVersion != meta.FormatVersion || nm.CreatedAt != meta.CreatedAt {
		t.Fatalf("fields not preserved %+v", nm)
	}
	if string(nm.Salt) == string(meta.Salt) {
		t.Fatal("salt not renewed")
	}
	got, err := unwrapVaultKey(c, kdf, nm, []byte("new"))
	if err != nil || got != vk {
		t.Fatalf("new pw: %v", err)
	}
	if _, err := unwrapVaultKey(c, kdf, nm, []byte("old")); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("old pw: %v", err)
	}
	if _, err := rewrapVaultKey(c, kdf, meta, []byte("bad"), []byte("x")); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("bad old pw: %v", err)
	}
}

func TestEnvKeyWrapBoundToEnvID(t *testing.T) {
	c := newTestCipher()
	vk, _ := c.NewKey()
	k, wrapped, err := newEnvKey(c, vk, "env1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := unwrapEnvKey(c, vk, "env1", wrapped)
	if err != nil || got != k {
		t.Fatalf("roundtrip: %v", err)
	}
	if _, err := unwrapEnvKey(c, vk, "env2", wrapped); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("other env id: %v", err)
	}
}

func TestTruncatedWrappedKeyIsCorrupt(t *testing.T) {
	c, kdf := newTestCipher(), newTestKDF()
	meta, _, _ := newVaultKeys(c, kdf, testParams, []byte("pw"))
	meta.WrappedVaultKey = meta.WrappedVaultKey[:5]
	if _, err := unwrapVaultKey(c, kdf, meta, []byte("pw")); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	vk, _ := c.NewKey()
	if _, err := unwrapEnvKey(c, vk, "e", []byte{1, 2}); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("env: %v", err)
	}
}

func TestUnwrapRejectsHostileKDFParams(t *testing.T) {
	c, kdf := newTestCipher(), newTestKDF()
	tests := []struct {
		name string
		p    domain.KDFParams
		ok   bool
	}{
		{"time0", domain.KDFParams{Time: 0, MemoryKiB: 8, Threads: 1}, false},
		{"threads0", domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 0}, false},
		{"mem0", domain.KDFParams{Time: 1, MemoryKiB: 0, Threads: 1}, false},
		{"memTooBig", domain.KDFParams{Time: 1, MemoryKiB: 1<<20 + 1, Threads: 1}, false},
		{"timeTooBig", domain.KDFParams{Time: 65, MemoryKiB: 8, Threads: 1}, false},
		{"timeHuge", domain.KDFParams{Time: 4294967295, MemoryKiB: 8, Threads: 1}, false},
		{"threadsTooBig", domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 65}, false},
		{"valid", domain.KDFParams{Time: 1, MemoryKiB: 1 << 20, Threads: 1}, true},
		{"defaults", domain.KDFParams{Time: 4, MemoryKiB: 256 * 1024, Threads: 4}, true},
		{"upperBounds", domain.KDFParams{Time: 64, MemoryKiB: 1 << 20, Threads: 64}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, _, err := newVaultKeys(c, kdf, testParams, []byte("pw"))
			if err != nil {
				t.Fatal(err)
			}
			meta.KDF = tt.p
			_, err = unwrapVaultKey(c, kdf, meta, []byte("pw"))
			_, err2 := rewrapVaultKey(c, kdf, meta, []byte("pw"), []byte("n"))
			if tt.ok {
				// Control: the fake KDF ignores params, so a valid set unwraps.
				if err != nil || err2 != nil {
					t.Fatalf("valid rejected: %v %v", err, err2)
				}
				return
			}
			if !errors.Is(err, domain.ErrCorrupt) || !errors.Is(err2, domain.ErrCorrupt) {
				t.Fatalf("got %v / %v", err, err2)
			}
		})
	}
}
