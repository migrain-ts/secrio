package domain

import "testing"

func TestNewIDUnique(t *testing.T) {
	seen := map[ID[Vault]]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID[Vault]()
		if len(id) != 32 {
			t.Fatalf("len %d", len(id))
		}
		if seen[id] {
			t.Fatal("duplicate id")
		}
		seen[id] = true
	}
}
