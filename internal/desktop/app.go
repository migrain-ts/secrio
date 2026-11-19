package desktop

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"

	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/domain"
)

// App is the facade bound to the desktop shell. Its methods are safe for
// concurrent use.
type App struct {
	deps Deps

	mu      sync.Mutex // guards svc and current
	svc     *app.Service
	current string // selected vault id
}

// New builds an App. Missing NewService or DataDir is a programmer error.
func New(d Deps) *App {
	if d.NewService == nil {
		panic("desktop.New: Deps.NewService is required")
	}
	if d.DataDir == nil {
		panic("desktop.New: Deps.DataDir is required")
	}
	if d.AfterFunc == nil {
		d.AfterFunc = defaultAfterFunc
	}
	return &App{deps: d}
}

// service returns the Service, creating it once.
func (a *App) service() (*app.Service, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.svc != nil {
		return a.svc, nil
	}
	svc, err := a.deps.NewService()
	if err != nil {
		return nil, toAPIError(err)
	}
	a.svc = svc
	return svc, nil
}

func (a *App) dataDir() (string, error) {
	dir, err := a.deps.DataDir()
	if err != nil {
		return "", toAPIError(err)
	}
	return dir, nil
}

// clearOurClipboard removes our copied secret from the clipboard. It is a
// documented no-op until the clipboard task implements it.
func (a *App) clearOurClipboard() {}

func (a *App) setCurrent(id string) {
	a.mu.Lock()
	a.current = id
	a.mu.Unlock()
}

func (a *App) getCurrent() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current
}

// selectEntry remembers id in memory and in desktop.json.
func (a *App) selectEntry(id string) error {
	dir, err := a.dataDir()
	if err != nil {
		return err
	}
	a.setCurrent(id)
	if err := savePrefs(dir, prefs{LastVault: id}); err != nil {
		return toAPIError(err)
	}
	return nil
}

func listEntries(ctx context.Context, svc *app.Service) ([]app.VaultEntry, error) {
	es, err := svc.ListVaults(ctx)
	if err != nil {
		return nil, toAPIError(err)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Name < es[j].Name })
	return es, nil
}

func findEntry(es []app.VaultEntry, id string) (app.VaultEntry, bool) {
	if id == "" {
		return app.VaultEntry{}, false
	}
	for _, e := range es {
		if e.ID == id {
			return e, true
		}
	}
	return app.VaultEntry{}, false
}

// resolveCurrent finds the current vault: the in-memory selection, else the
// remembered one, else the only vault. ok is false when there is none.
func (a *App) resolveCurrent(ctx context.Context, svc *app.Service) (app.VaultEntry, bool, error) {
	es, err := listEntries(ctx, svc)
	if err != nil {
		return app.VaultEntry{}, false, err
	}
	if e, ok := findEntry(es, a.getCurrent()); ok {
		return e, true, nil
	}
	a.setCurrent("")
	dir, err := a.dataDir()
	if err != nil {
		return app.VaultEntry{}, false, err
	}
	if e, ok := findEntry(es, loadPrefs(dir).LastVault); ok {
		a.setCurrent(e.ID)
		return e, true, nil
	}
	if len(es) == 1 {
		if err := a.selectEntry(es[0].ID); err != nil {
			return app.VaultEntry{}, false, err
		}
		return es[0], true, nil
	}
	return app.VaultEntry{}, false, nil
}

// withVault opens the current vault, runs fn and always closes the handle.
func (a *App) withVault(fn func(ctx context.Context, svc *app.Service, entry app.VaultEntry, u *app.Unlocked) error) (err error) {
	svc, err := a.service()
	if err != nil {
		return err
	}
	ctx := context.Background()
	entry, ok, err := a.resolveCurrent(ctx, svc)
	if err != nil {
		return err
	}
	if !ok {
		return newAPIError(CodeNoVault, "no vault selected")
	}
	u, err := svc.Open(ctx, entry.ID)
	if err != nil {
		return toAPIError(err)
	}
	defer func() {
		if cerr := u.Close(); err == nil {
			err = toAPIError(cerr)
		}
	}()
	return toAPIError(fn(ctx, svc, entry, u))
}

// ListVaults returns the registered vaults sorted by name.
func (a *App) ListVaults() ([]VaultDTO, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	es, err := listEntries(context.Background(), svc)
	if err != nil {
		return nil, err
	}
	out := make([]VaultDTO, len(es))
	for i, e := range es {
		out[i] = VaultDTO{ID: e.ID, Name: e.Name}
	}
	return out, nil
}

var vaultNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// validVaultName mirrors the CLI naming rule (copied, not imported).
func validVaultName(name string) bool {
	return vaultNameRE.MatchString(name) && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "-") &&
		!strings.Contains(name, "..") && !windowsReserved(name)
}

func windowsReserved(name string) bool {
	base, _, _ := strings.Cut(strings.ToLower(name), ".")
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(base) == 4 && (base[:3] == "com" || base[:3] == "lpt") && base[3] >= '1' && base[3] <= '9'
}

// CreateVault creates a vault, unlocks it and selects it.
func (a *App) CreateVault(name, password string) (VaultDTO, error) {
	if !validVaultName(name) {
		return VaultDTO{}, newAPIError(CodeInvalidName, "invalid name")
	}
	if password == "" {
		return VaultDTO{}, newAPIError(CodeInvalidValue, "password must not be empty")
	}
	svc, err := a.service()
	if err != nil {
		return VaultDTO{}, err
	}
	ctx := context.Background()
	es, err := listEntries(ctx, svc)
	if err != nil {
		return VaultDTO{}, err
	}
	for _, e := range es {
		if strings.EqualFold(e.Name, name) {
			return VaultDTO{}, newAPIError(CodeDuplicate, "already exists")
		}
	}
	dir, err := a.dataDir()
	if err != nil {
		return VaultDTO{}, err
	}
	path, err := core.VaultPath(dir, name)
	if err != nil {
		return VaultDTO{}, toAPIError(err)
	}
	pw := []byte(password)
	defer clear(pw)
	v, err := svc.CreateVault(ctx, name, path, pw)
	if err != nil {
		return VaultDTO{}, toAPIError(err)
	}
	id := string(v.ID)
	if err := svc.Unlock(ctx, id, pw, sessionTTL, sessionIdle); err != nil {
		return VaultDTO{}, toAPIError(err)
	}
	if err := a.selectEntry(id); err != nil {
		return VaultDTO{}, err
	}
	return VaultDTO{ID: id, Name: name}, nil
}

// Unlock opens a session for vaultID and selects it.
func (a *App) Unlock(vaultID, password string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	pw := []byte(password)
	defer clear(pw)
	if err := svc.Unlock(context.Background(), vaultID, pw, sessionTTL, sessionIdle); err != nil {
		return toAPIError(err)
	}
	return a.selectEntry(vaultID)
}

// Lock ends the current vault's session and clears our clipboard copy.
func (a *App) Lock() error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	ctx := context.Background()
	e, ok, err := a.resolveCurrent(ctx, svc)
	if err != nil {
		return err
	}
	if !ok {
		return newAPIError(CodeNoVault, "no vault selected")
	}
	lerr := svc.Lock(ctx, e.ID)
	a.clearOurClipboard()
	return toAPIError(lerr)
}

// SelectVault makes vaultID current and remembers it.
func (a *App) SelectVault(vaultID string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	es, err := listEntries(context.Background(), svc)
	if err != nil {
		return err
	}
	if _, ok := findEntry(es, vaultID); !ok {
		return newAPIError(CodeNotFound, "not found")
	}
	return a.selectEntry(vaultID)
}

// CurrentVault reports the current vault and whether its session is open.
func (a *App) CurrentVault() (VaultDTO, bool, error) {
	svc, err := a.service()
	if err != nil {
		return VaultDTO{}, false, err
	}
	ctx := context.Background()
	e, ok, err := a.resolveCurrent(ctx, svc)
	if err != nil || !ok {
		return VaultDTO{}, false, err
	}
	dto := VaultDTO{ID: e.ID, Name: e.Name}
	u, err := svc.Open(ctx, e.ID)
	if errors.Is(err, domain.ErrLocked) {
		return dto, false, nil
	}
	if err != nil {
		return VaultDTO{}, false, toAPIError(err)
	}
	_ = u.Close()
	return dto, true, nil
}

// Shutdown locks the current vault and clears our clipboard copy. It is
// idempotent and never fails.
func (a *App) Shutdown() {
	defer func() { _ = recover() }()
	if svc, err := a.service(); err == nil {
		ctx := context.Background()
		if e, ok, err := a.resolveCurrent(ctx, svc); err == nil && ok {
			_ = svc.Lock(ctx, e.ID)
		}
	}
	a.clearOurClipboard()
}
