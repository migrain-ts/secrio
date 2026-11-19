package desktop

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClip is a settable clipboard that logs its calls.
type fakeClip struct {
	mu     sync.Mutex
	text   string
	getErr error
	setErr error
	gets   int
	sets   []string // every SetText argument, in order
}

func (c *fakeClip) GetText() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	if c.getErr != nil {
		return "", c.getErr
	}
	return c.text, nil
}

func (c *fakeClip) SetText(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	c.sets = append(c.sets, s)
	c.text = s
	return nil
}

// set simulates another app writing the clipboard.
func (c *fakeClip) set(s string) {
	c.mu.Lock()
	c.text = s
	c.mu.Unlock()
}

func (c *fakeClip) current() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
}

// clears counts SetText("") calls.
func (c *fakeClip) clears() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, s := range c.sets {
		if s == "" {
			n++
		}
	}
	return n
}

func (c *fakeClip) setCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sets)
}

type fakeTimer struct {
	d       time.Duration
	f       func()
	stopped bool
	fired   bool
}

// fakeTimers is an AfterFunc whose timers the test fires by hand.
type fakeTimers struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (ft *fakeTimers) AfterFunc(d time.Duration, f func()) func() bool {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	t := &fakeTimer{d: d, f: f}
	ft.timers = append(ft.timers, t)
	return func() bool {
		ft.mu.Lock()
		defer ft.mu.Unlock()
		if t.stopped || t.fired {
			return false
		}
		t.stopped = true
		return true
	}
}

func (ft *fakeTimers) count() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return len(ft.timers)
}

func (ft *fakeTimers) get(i int) fakeTimer {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return *ft.timers[i]
}

// fire runs timer i even if stopped (a stopped timer that was already
// running must still be harmless).
func (ft *fakeTimers) fire(i int) {
	ft.mu.Lock()
	t := ft.timers[i]
	t.fired = true
	ft.mu.Unlock()
	t.f()
}

type clipEnv struct {
	*testEnv
	clip   *fakeClip
	timers *fakeTimers
	envID  string
}

const (
	secretA = "value-A-marker-7c1e"
	secretB = "value-B-marker-2d9f"
)

func newClipEnv(t *testing.T) *clipEnv {
	t.Helper()
	c := &clipEnv{clip: &fakeClip{text: "before"}, timers: &fakeTimers{}}
	c.testEnv = newTestAppWith(t, func(d *Deps) {
		d.Clipboard = c.clip
		d.AfterFunc = c.timers.AfterFunc
	})
	mustCreate(t, c.testEnv, "work")
	p, err := c.app.CreateProject("web")
	if err != nil {
		t.Fatal(err)
	}
	e, err := c.app.CreateEnvironment(p.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	c.envID = e.ID
	for n, v := range map[string]string{"A": secretA, "B": secretB} {
		if err := c.app.SaveSecret(c.envID, n, v, ""); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func (c *clipEnv) copy(t *testing.T, name string, want int) {
	t.Helper()
	r, err := c.app.CopySecret(c.envID, name)
	if err != nil {
		t.Fatal(err)
	}
	if r.ClearInSeconds != want {
		t.Fatalf("ClearInSeconds=%d want %d", r.ClearInSeconds, want)
	}
}

func TestCopySetsTextAndArms(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	if c.clip.current() != secretA {
		t.Fatal("clipboard not set")
	}
	if c.timers.count() != 1 || c.timers.get(0).d != 30*time.Second {
		t.Fatalf("timers: %d", c.timers.count())
	}
}

func TestFireClearsWhenUnchanged(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.timers.fire(0)
	if c.clip.current() != "" || c.clip.clears() != 1 {
		t.Fatalf("want cleared, got clears=%d", c.clip.clears())
	}
	// State reset: Lock does not clear again.
	_ = c.app.Lock()
	if c.clip.clears() != 1 {
		t.Fatal("cleared twice")
	}
}

func TestFireKeepsOtherAppsCopy(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.clip.set("user copied this elsewhere")
	c.timers.fire(0)
	if c.clip.current() != "user copied this elsewhere" || c.clip.clears() != 0 {
		t.Fatal("must not clear another app's copy")
	}
	// State reset: even if our value comes back, Lock leaves it.
	c.clip.set(secretA)
	_ = c.app.Lock()
	if c.clip.clears() != 0 {
		t.Fatal("state not reset after fire")
	}
}

func TestCopyTwiceRearms(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.copy(t, "B", 30)
	if c.timers.count() != 2 || !c.timers.get(0).stopped || c.timers.get(1).stopped {
		t.Fatalf("first timer must be stopped, second armed")
	}
	if c.clip.current() != secretB {
		t.Fatal("second copy not set")
	}
	// A stale first timer that was already running does nothing.
	c.timers.fire(0)
	if c.clip.clears() != 0 {
		t.Fatal("stale timer cleared")
	}
	// The second timer clears B (the second hash replaced the first).
	c.timers.fire(1)
	if c.clip.current() != "" || c.clip.clears() != 1 {
		t.Fatal("second timer must clear")
	}
}

func TestSecondHashReplacesFirst(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.copy(t, "B", 30)
	c.clip.set(secretA) // the first value is no longer ours
	c.timers.fire(1)
	if c.clip.clears() != 0 || c.clip.current() != secretA {
		t.Fatal("old hash must be forgotten")
	}
}

func TestLockAndShutdownClear(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(*App)
	}{
		{"lock", func(a *App) { _ = a.Lock() }},
		{"shutdown", func(a *App) { a.Shutdown() }},
	} {
		t.Run(tc.name+"/ours", func(t *testing.T) {
			c := newClipEnv(t)
			c.copy(t, "A", 30)
			tc.do(c.app)
			if c.clip.current() != "" || c.clip.clears() != 1 {
				t.Fatal("must clear our copy")
			}
			if !c.timers.get(0).stopped {
				t.Fatal("timer must be stopped")
			}
		})
		t.Run(tc.name+"/theirs", func(t *testing.T) {
			c := newClipEnv(t)
			c.copy(t, "A", 30)
			c.clip.set("theirs")
			tc.do(c.app)
			if c.clip.current() != "theirs" || c.clip.clears() != 0 {
				t.Fatal("must not clear another app's copy")
			}
		})
	}
}

func TestUnreadableClipboard(t *testing.T) {
	c := newClipEnv(t)
	c.clip.getErr = ErrClipboardUnreadable
	c.copy(t, "A", 0)
	if c.clip.current() != secretA {
		t.Fatal("copy itself must succeed")
	}
	if c.timers.count() != 0 {
		t.Fatal("no timer when unreadable")
	}
	c.clip.getErr = nil // even if it becomes readable later
	_ = c.app.Lock()
	c.app.Shutdown()
	if c.clip.clears() != 0 {
		t.Fatal("never clear an unverified copy")
	}
}

func TestReadBackMismatch(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	// A clipboard that alters the text on write.
	mc := &mangleClip{fakeClip: c.clip}
	c.app.deps.Clipboard = mc
	c.copy(t, "B", 0)
	if !c.timers.get(0).stopped || c.timers.count() != 1 {
		t.Fatal("previous timer must be stopped and no new one armed")
	}
	_ = c.app.Lock()
	if c.clip.clears() != 0 {
		t.Fatal("must not clear after mismatch")
	}
}

type mangleClip struct{ *fakeClip }

func (m *mangleClip) SetText(s string) error { return m.fakeClip.SetText(s + "\r\n") }

func TestGetErrorAtFire(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	c.clip.mu.Lock()
	c.clip.getErr = errors.New("busy")
	c.clip.mu.Unlock()
	c.timers.fire(0)
	c.clip.mu.Lock()
	c.clip.getErr = nil
	c.clip.mu.Unlock()
	_ = c.app.Lock()
	if c.clip.clears() != 0 || c.clip.current() != secretA {
		t.Fatal("must never blind-clear, and state is reset after fire")
	}
}

func TestCopyUnknownSecret(t *testing.T) {
	c := newClipEnv(t)
	_, err := c.app.CopySecret(c.envID, "MISSING")
	if err == nil || codeOf(t, err) != CodeNotFound {
		t.Fatalf("got %v", err)
	}
	_, err = c.app.CopySecret("no-such-env", "A")
	if err == nil || codeOf(t, err) != CodeNotFound {
		t.Fatalf("env: %v", err)
	}
	_, err = c.app.CopySecret(c.envID, "bad name")
	if err == nil || codeOf(t, err) != CodeInvalidName {
		t.Fatalf("name: %v", err)
	}
	if c.clip.setCount() != 0 || c.clip.current() != "before" || c.timers.count() != 0 {
		t.Fatal("clipboard must be untouched")
	}
}

func TestCopySetTextFailure(t *testing.T) {
	c := newClipEnv(t)
	c.clip.setErr = errors.New("cannot write " + secretA)
	_, err := c.app.CopySecret(c.envID, "A")
	if err == nil || codeOf(t, err) != CodeInternal {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), secretA) || strings.Contains(err.Error(), "cannot write") {
		t.Fatalf("error leaks: %s", err)
	}
	if c.timers.count() != 0 {
		t.Fatal("no timer on failure")
	}
}

func TestCopyNilClipboard(t *testing.T) {
	env, id := secretEnv(t)
	_, err := env.app.CopySecret(id, "A")
	if err == nil || codeOf(t, err) != CodeUnsupported {
		t.Fatalf("got %v", err)
	}
	env.app.clearOurClipboard() // safe with nil
	_ = env.app.Lock()
	env.app.Shutdown()
}

func TestCopyLocked(t *testing.T) {
	c := newClipEnv(t)
	_ = c.app.Lock()
	_, err := c.app.CopySecret(c.envID, "A")
	if err == nil || codeOf(t, err) != CodeLocked {
		t.Fatalf("got %v", err)
	}
	if c.clip.setCount() != 0 {
		t.Fatal("clipboard touched")
	}
}

// containsString walks v's strings and byte slices (structs, arrays,
// slices, maps and pointers to structs inside the App, not the deps'
// collaborators) looking for needle.
func containsString(v reflect.Value, needle string, seen map[uintptr]bool) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), needle)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return strings.Contains(string(v.Bytes()), needle)
		}
		for i := 0; i < v.Len(); i++ {
			if containsString(v.Index(i), needle, seen) {
				return true
			}
		}
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, v.Len())
			for i := range b {
				b[i] = byte(v.Index(i).Uint())
			}
			return strings.Contains(string(b), needle)
		}
		for i := 0; i < v.Len(); i++ {
			if containsString(v.Index(i), needle, seen) {
				return true
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			if containsString(it.Key(), needle, seen) || containsString(it.Value(), needle, seen) {
				return true
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if containsString(v.Field(i), needle, seen) {
				return true
			}
		}
	}
	return false
}

func TestCopyDoesNotRetainValue(t *testing.T) {
	c := newClipEnv(t)
	c.copy(t, "A", 30)
	if s := fmt.Sprintf("%+v %#v", c.app, c.app); strings.Contains(s, secretA) {
		t.Fatal("value in App dump")
	}
	v := reflect.ValueOf(c.app).Elem()
	if containsString(v, secretA, map[uintptr]bool{}) {
		t.Fatal("value retained in App fields")
	}
	// Self-check of the walker: it does see strings in the App.
	if !containsString(v, c.app.getCurrent(), map[uintptr]bool{}) {
		t.Fatal("walker blind")
	}
}

func TestConcurrentCopyAndLock(t *testing.T) {
	c := newClipEnv(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "A"
			if i%2 == 1 {
				name = "B"
			}
			_, _ = c.app.CopySecret(c.envID, name)
			if i%3 == 0 {
				_ = c.app.Lock()
				_ = c.app.Unlock(c.app.getCurrent(), testPassword)
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n := c.timers.count(); n > 0 {
				c.timers.fire(n - 1)
			}
		}()
	}
	wg.Wait()
	c.app.Shutdown()
}
