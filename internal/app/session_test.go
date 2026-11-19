package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"secrio/internal/domain"
)

// assertNoSession fails unless both halves of the session of id are gone.
func (h *harness) assertNoSession(t *testing.T, id string) {
	t.Helper()
	if h.vault().session != nil {
		t.Error("session row not deleted")
	}
	if _, ok := h.keys.m[id]; ok {
		t.Error("session key not deleted")
	}
}

// assertLocked fails unless err is ErrLocked and none of the lower-level
// session defects leak through.
func assertLocked(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrLocked) || errors.Is(err, domain.ErrCorrupt) || errors.Is(err, domain.ErrAuthFailed) {
		t.Fatalf("got %v, want ErrLocked only", err)
	}
}

// vaultKey returns the real vault key via the password path.
func (h *harness) vaultKey(t *testing.T, id string) domain.Key {
	t.Helper()
	u, err := h.svc.OpenWithPassword(context.Background(), id, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	return u.key
}

func (h *harness) unlock(t *testing.T, id string, ttl, idle time.Duration) {
	t.Helper()
	if err := h.svc.Unlock(context.Background(), id, []byte(testPw), ttl, idle); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockThenOpen(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)

	row := h.vault().session
	if row == nil {
		t.Fatal("no session row")
	}
	sk, ok := h.keys.m[id]
	if !ok || sk == (domain.Key{}) {
		t.Fatal("no session key")
	}
	vk := h.vaultKey(t, id)
	if sk == vk || bytes.Contains(row.Blob, vk[:]) {
		t.Fatal("vault key stored in the clear")
	}
	now := h.clock.Now()
	if !row.ExpiresAt.Equal(now.Add(time.Hour)) || !row.LastUsed.Equal(now) || row.Idle != 0 {
		t.Fatalf("row timing = %v %v %v", row.ExpiresAt, row.LastUsed, row.Idle)
	}

	u, err := h.svc.Open(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if u.key != vk {
		t.Fatal("handle does not hold the vault key")
	}
	if _, err := u.CreateProject(ctx, "api", ""); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockNonPositiveTTL(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	for _, ttl := range []time.Duration{0, -time.Second} {
		err := h.svc.Unlock(ctx, id, []byte(testPw), ttl, 0)
		if err == nil || errors.Is(err, domain.ErrLocked) {
			t.Fatalf("ttl %v: %v", ttl, err)
		}
		// Checked before anything else: even an unknown vault reports it.
		if err := h.svc.Unlock(ctx, "nope", []byte(testPw), ttl, 0); err == nil || errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("ttl %v unknown vault: %v", ttl, err)
		}
	}
	h.assertNoSession(t, id)
}

func TestOpenAfterLockIsLocked(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	if err := h.svc.Lock(ctx, id); err != nil {
		t.Fatal(err)
	}
	h.assertNoSession(t, id)
	_, err := h.svc.Open(ctx, id)
	assertLocked(t, err)
}

func TestLockIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	if err := h.svc.Lock(ctx, id); err != nil { // no session at all
		t.Fatalf("lock without session: %v", err)
	}
	h.unlock(t, id, time.Hour, 0)
	for i := 0; i < 2; i++ {
		if err := h.svc.Lock(ctx, id); err != nil {
			t.Fatalf("lock %d: %v", i, err)
		}
	}
	h.assertNoSession(t, id)
}

func TestSessionExpiresAtTTL(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	h.clock.Advance(59 * time.Minute)
	u, err := h.svc.Open(ctx, id)
	if err != nil {
		t.Fatalf("before ttl: %v", err)
	}
	u.Close()
	h.clock.Advance(time.Minute)
	_, err = h.svc.Open(ctx, id)
	assertLocked(t, err)
	h.assertNoSession(t, id)
}

func TestSessionIdleTimeout(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, 8*time.Hour, 10*time.Minute)
	for i := 0; i < 3; i++ { // each use refreshes LastUsed
		h.clock.Advance(9 * time.Minute)
		u, err := h.svc.Open(ctx, id)
		if err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
		u.Close()
	}
	if got := h.vault().session.LastUsed; !got.Equal(h.clock.Now()) {
		t.Fatalf("LastUsed not refreshed: %v", got)
	}
	h.clock.Advance(10 * time.Minute)
	_, err := h.svc.Open(ctx, id)
	assertLocked(t, err)
	h.assertNoSession(t, id)
}

func TestOpenRefreshesLastUsedNotExpiry(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	start := h.clock.Now()
	h.unlock(t, id, time.Hour, 30*time.Minute)
	h.clock.Advance(10 * time.Minute)
	u, err := h.svc.Open(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	u.Close()
	row := h.vault().session
	if !row.LastUsed.Equal(h.clock.Now()) {
		t.Fatalf("LastUsed = %v, want %v", row.LastUsed, h.clock.Now())
	}
	if !row.ExpiresAt.Equal(start.Add(time.Hour)) || row.Idle != 30*time.Minute {
		t.Fatalf("ExpiresAt/Idle changed: %v %v", row.ExpiresAt, row.Idle)
	}
}

func TestRowWithoutKeyIsLocked(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	delete(h.keys.m, id)
	_, err := h.svc.Open(context.Background(), id)
	assertLocked(t, err)
	h.assertNoSession(t, id)
}

func TestKeyWithoutRowIsLocked(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	h.vault().session = nil
	_, err := h.svc.Open(context.Background(), id)
	assertLocked(t, err)
	h.assertNoSession(t, id)
}

func TestTamperedSessionBlobIsLocked(t *testing.T) {
	for name, tamper := range map[string]func([]byte) []byte{
		"flipped":   func(b []byte) []byte { b[len(b)/2] ^= 0xff; return b },
		"truncated": func(b []byte) []byte { return b[:4] },
		"empty":     func([]byte) []byte { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			id := h.newVault(t)
			h.unlock(t, id, time.Hour, 0)
			h.vault().session.Blob = tamper(h.vault().session.Blob)
			_, err := h.svc.Open(context.Background(), id)
			assertLocked(t, err)
			h.assertNoSession(t, id)
		})
	}
}

func TestSessionKeyFromOtherVaultIsLocked(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	idA := h.newVault(t)
	vb, err := h.svc.CreateVault(ctx, "b", "/vaults/b.db", []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	idB := string(vb.ID)
	h.unlock(t, idA, time.Hour, 0)
	h.unlock(t, idB, time.Hour, 0)

	// Graft A's whole session (row and key) onto B. Without the vault id in
	// the AAD, B would open with A's vault key.
	rowA := *h.vault().session
	h.opener.files["/vaults/b.db"].session = &rowA
	h.keys.m[idB] = h.keys.m[idA]
	_, err = h.svc.Open(ctx, idB)
	assertLocked(t, err)
	if h.opener.files["/vaults/b.db"].session != nil {
		t.Error("grafted row not deleted")
	}
	if _, ok := h.keys.m[idB]; ok {
		t.Error("grafted key not deleted")
	}
	u, err := h.svc.Open(ctx, idA) // A itself is untouched
	if err != nil {
		t.Fatal(err)
	}
	u.Close()
}

func TestChangePasswordKeepsSessionValid(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	if err := h.svc.ChangePassword(ctx, id, []byte(testPw), []byte("new password")); err != nil {
		t.Fatal(err)
	}
	u, err := h.svc.Open(ctx, id)
	if err != nil {
		t.Fatalf("session after password change: %v", err)
	}
	u.Close()
}

func TestKeychainNeverReceivesVaultKey(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	vk := h.vaultKey(t, id)
	for i := 0; i < 3; i++ {
		h.unlock(t, id, time.Hour, 0)
		u, err := h.svc.Open(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		u.Close()
	}
	if len(h.keys.saved) != 3 {
		t.Fatalf("%d keys saved, want 3", len(h.keys.saved))
	}
	seen := map[domain.Key]bool{}
	for _, k := range h.keys.saved {
		if k == vk || k == (domain.Key{}) || seen[k] {
			t.Fatal("keychain received the vault key, a zero key or a reused key")
		}
		seen[k] = true
	}
}

func TestUnlockKeysSaveFailureRemovesRow(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	down := errors.New("keychain down")
	h.keys.saveErr = down
	if err := h.svc.Unlock(context.Background(), id, []byte(testPw), time.Hour, 0); !errors.Is(err, down) {
		t.Fatalf("got %v", err)
	}
	h.assertNoSession(t, id)
}

func TestOpenKeysLoadErrorIsReturnedAndRowKept(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	unavailable := errors.New("keychain unavailable")
	h.keys.loadErr = unavailable
	_, err := h.svc.Open(ctx, id)
	if !errors.Is(err, unavailable) || errors.Is(err, domain.ErrLocked) {
		t.Fatalf("got %v", err)
	}
	if h.vault().session == nil {
		t.Fatal("row deleted on a transient keychain error")
	}
	h.keys.loadErr = nil
	u, err := h.svc.Open(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	u.Close()
}

// gatedOpener hands out stores whose first TouchSession (once armed) signals
// touching and waits for release before running.
type gatedOpener struct {
	*fakeOpener
	armed    bool
	touching chan struct{}
	release  chan struct{}
}

type gatedStore struct {
	RecordStore
	g *gatedOpener
}

func (o *gatedOpener) Open(ctx context.Context, path string) (RecordStore, error) {
	st, err := o.fakeOpener.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return &gatedStore{RecordStore: st, g: o}, nil
}

func (s *gatedStore) TouchSession(ctx context.Context, now time.Time) (bool, error) {
	if s.g.armed {
		s.g.armed = false
		close(s.g.touching)
		<-s.g.release
	}
	return s.RecordStore.TouchSession(ctx, now)
}

func TestLockNotUndoneByConcurrentOpen(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	g := &gatedOpener{fakeOpener: h.opener, touching: make(chan struct{}), release: make(chan struct{})}
	h.svc = NewService(newTestCipher(), newTestKDF(), testParams, g, h.reg, h.keys, h.clock)
	id := h.newVault(t)
	h.unlock(t, id, time.Hour, 0)
	g.armed = true

	opened := make(chan error)
	go func() { // Open has loaded the session key and pauses before Touch
		u, err := h.svc.Open(ctx, id)
		if u != nil {
			u.Close()
		}
		opened <- err
	}()
	<-g.touching
	if err := h.svc.Lock(ctx, id); err != nil { // nothing serialises: completes now
		t.Fatal(err)
	}
	close(g.release)
	assertLocked(t, <-opened)
	_, err := h.svc.Open(ctx, id)
	assertLocked(t, err)
	h.assertNoSession(t, id)
}
