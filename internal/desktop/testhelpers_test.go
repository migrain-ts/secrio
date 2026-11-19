package desktop

import (
	"errors"
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

type testEnv struct {
	app   *App
	dir   string
	fake  *clock.Fake
	keys  *keychain.MemoryKeyStore
	newed int // how many times NewService ran
}

// newTestApp builds an App over a real Core in a temp dir.
func newTestApp(t *testing.T) *testEnv {
	t.Helper()
	env := &testEnv{
		dir:  t.TempDir(),
		fake: clock.NewFake(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)),
		keys: keychain.NewMemory(),
	}
	env.app = New(Deps{
		DataDir: func() (string, error) { return env.dir, nil },
		NewService: func() (*app.Service, error) {
			env.newed++
			return env.otherService()
		},
	})
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
