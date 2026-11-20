package desktop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
	"secrio/internal/infra/clock"
	"secrio/internal/infra/keychain"
)

var cheapKDF = domain.KDFParams{Time: 1, MemoryKiB: 8, Threads: 1}

const testPassword = "correct horse battery staple"

// countingKeys counts Load calls, one per Open of a vault.
type countingKeys struct {
	*keychain.MemoryKeyStore
	loads atomic.Int32

	failSave    atomic.Bool // Save fails
	panicDelete atomic.Bool // Delete panics (a crash in the lock path)
	onSave      func()      // runs at the start of Save; set before use
	failDelete  string      // Delete fails for this vault id; set before use
}

func (c *countingKeys) Save(ctx context.Context, id string, k domain.Key) error {
	if c.onSave != nil {
		c.onSave()
	}
	if c.failSave.Load() {
		return errors.New("keychain unavailable")
	}
	return c.MemoryKeyStore.Save(ctx, id, k)
}

func (c *countingKeys) Delete(ctx context.Context, id string) error {
	if c.panicDelete.Load() {
		panic("injected")
	}
	if c.failDelete != "" && id == c.failDelete {
		return errors.New("keychain unavailable")
	}
	return c.MemoryKeyStore.Delete(ctx, id)
}

func (c *countingKeys) Load(ctx context.Context, id string) (domain.Key, error) {
	c.loads.Add(1)
	return c.MemoryKeyStore.Load(ctx, id)
}

type testEnv struct {
	app   *App
	dir   string
	fake  *clock.Fake
	keys  *countingKeys
	newed int // how many times NewService ran
}

// newTestApp builds an App over a real Core in a temp dir.
func newTestApp(t *testing.T) *testEnv { return newTestAppWith(t, nil) }

// newTestAppWith is newTestApp with a hook to adjust the Deps.
func newTestAppWith(t *testing.T, opt func(*Deps)) *testEnv {
	t.Helper()
	env := &testEnv{
		dir:  t.TempDir(),
		fake: clock.NewFake(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)),
		keys: &countingKeys{MemoryKeyStore: keychain.NewMemory()},
	}
	d := Deps{
		DataDir: func() (string, error) { return env.dir, nil },
		NewService: func() (*app.Service, error) {
			env.newed++
			return env.otherService()
		},
	}
	if opt != nil {
		opt(&d)
	}
	env.app = New(d)
	return env
}

// otherService builds another Service on the same dir and key store (as the
// CLI would).
func (e *testEnv) otherService() (*app.Service, error) {
	return core.New(e.dir, core.WithKDFParams(cheapKDF), core.WithSessionKeys(e.keys), core.WithClock(e.fake))
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ae *apiError
	if !errors.As(err, &ae) {
		t.Fatalf("not an apiError: %v", err)
	}
	return ae.Code
}
