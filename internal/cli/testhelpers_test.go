package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
	"secrio/internal/infra/clock"
	"secrio/internal/infra/keychain"
)

// cheapKDF keeps tests fast.
var cheapKDF = domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 1}

const testPassword = "correct horse battery staple"

// newTestCore builds a real Core in a temp data dir with an in-memory
// session keystore and a fake clock.
func newTestCore(t *testing.T) (*app.Service, string, *clock.Fake) {
	t.Helper()
	dir := t.TempDir()
	fake := clock.NewFake(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC))
	svc, err := core.New(dir,
		core.WithKDFParams(cheapKDF),
		core.WithSessionKeys(keychain.NewMemory()),
		core.WithClock(fake),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, dir, fake
}

// createVault creates a vault named name under dir and returns its id.
func createVault(t *testing.T, svc *app.Service, dir, name string) string {
	t.Helper()
	v, err := svc.CreateVault(context.Background(), name, filepath.Join(dir, name+".db"), []byte(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	return string(v.ID)
}

// openUnlocked unlocks and opens the vault; cleanup closes it.
func openUnlocked(t *testing.T, svc *app.Service, id string) *app.Unlocked {
	t.Helper()
	ctx := context.Background()
	if err := svc.Unlock(ctx, id, []byte(testPassword), time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	u, err := svc.Open(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u.Close() })
	return u
}
