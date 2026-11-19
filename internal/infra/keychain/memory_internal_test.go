package keychain

import (
	"context"
	"testing"
)

func TestMemoryDeleteZeroesStoredKey(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	if err := s.Save(ctx, "v", sampleKey()); err != nil {
		t.Fatal(err)
	}
	stored := s.m["v"]
	if err := s.Delete(ctx, "v"); err != nil {
		t.Fatal(err)
	}
	if *stored != ([32]byte{}) {
		t.Fatal("stored key not zeroed on delete")
	}
	if _, ok := s.m["v"]; ok {
		t.Fatal("entry kept")
	}
	if err := s.Delete(ctx, "v"); err != nil { // idempotent
		t.Fatal(err)
	}
}
