package desktop

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// clearAfter is how long a copied value stays on the clipboard.
const clearAfter = 30 * time.Second

// clipState remembers our last copy by hash only, never the plaintext.
type clipState struct {
	mu   sync.Mutex
	hash [32]byte
	has  bool
	gen  uint64      // bumped on every arm; a stale timer sees a newer gen
	stop func() bool // stops the pending timer, nil when none
}

// disarmLocked stops the timer and forgets the copy. c.mu must be held.
func (c *clipState) disarmLocked() {
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.hash = [32]byte{}
	c.has = false
}

// clearIfOursLocked empties the clipboard only if it still holds our copy,
// then forgets it. An unreadable clipboard is never cleared. c.mu must be
// held.
func (a *App) clearIfOursLocked() {
	c := &a.clip
	if c.has && a.deps.Clipboard != nil {
		if cur, err := a.deps.Clipboard.GetText(); err == nil && sha256.Sum256([]byte(cur)) == c.hash {
			_ = a.deps.Clipboard.SetText("")
		}
	}
	c.disarmLocked()
}

// clearOurClipboard removes our copied secret from the clipboard if it is
// still there. Idempotent; safe with a nil Clipboard.
func (a *App) clearOurClipboard() {
	a.clip.mu.Lock()
	defer a.clip.mu.Unlock()
	a.clearIfOursLocked()
}

// fireClear is the timer callback for the copy armed as gen.
func (a *App) fireClear(gen uint64) {
	a.clip.mu.Lock()
	defer a.clip.mu.Unlock()
	if a.clip.gen != gen {
		return // stopped and replaced by a newer copy
	}
	a.clip.stop = nil // this timer has fired
	a.clearIfOursLocked()
}

// CopySecret puts a secret value on the clipboard (in Go; it never reaches
// the frontend) and arranges for it to be cleared after clearAfter if the
// clipboard still holds it. ClearInSeconds is 0 when the clipboard cannot be
// read back, in which case it is never cleared by us.
func (a *App) CopySecret(envID, name string) (CopyResult, error) {
	cb := a.deps.Clipboard
	if cb == nil {
		return CopyResult{}, unsupported()
	}
	n, err := domain.ParseSecretName(name)
	if err != nil {
		return CopyResult{}, newAPIError(CodeInvalidName, "invalid name")
	}
	var res CopyResult
	err = a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		s, err := u.GetSecret(ctx, eid(envID), n)
		if err != nil {
			return err
		}
		h := sha256.Sum256(s.Value)
		text := string(s.Value)
		clear(s.Value)

		c := &a.clip
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := cb.SetText(text); err != nil {
			return newAPIError(CodeInternal, "could not write the clipboard")
		}
		c.disarmLocked() // the previous copy, if any, was just overwritten
		got, err := cb.GetText()
		if err != nil || sha256.Sum256([]byte(got)) != h {
			return nil // copied, but we cannot verify it later: never clear
		}
		c.hash, c.has = h, true
		c.gen++
		gen := c.gen
		c.stop = a.deps.AfterFunc(clearAfter, func() { a.fireClear(gen) })
		res = CopyResult{ClearInSeconds: int(clearAfter / time.Second)}
		return nil
	})
	if err != nil {
		return CopyResult{}, err
	}
	return res, nil
}
