package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"slices"

	"secrio/internal/app"
	"secrio/internal/app/importexport"
	"secrio/internal/domain"
)

// Bounds of the remembered picked paths and previews.
const (
	pickedCap  = 16
	previewCap = 16
)

type previewKey struct{ envID, path string }

// importState is the facade's whole import memory: paths the user picked and
// one SHA-256 per preview. Never values. Guarded by App.mu.
type importState struct {
	picked   []string
	previews map[previewKey][32]byte
	order    []previewKey // insertion order, for dropping the oldest
}

func (s *importState) pick(p string) {
	if slices.Contains(s.picked, p) {
		return
	}
	if len(s.picked) >= pickedCap {
		s.picked = s.picked[1:]
	}
	s.picked = append(s.picked, p)
}

func (s *importState) put(k previewKey, h [32]byte) {
	if s.previews == nil {
		s.previews = map[previewKey][32]byte{}
	}
	if _, ok := s.previews[k]; !ok {
		if len(s.order) >= previewCap {
			delete(s.previews, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, k)
	}
	s.previews[k] = h
}

func (s *importState) drop(k previewKey) {
	delete(s.previews, k)
	if i := slices.Index(s.order, k); i >= 0 {
		s.order = slices.Delete(s.order, i, i+1)
	}
}

func (s *importState) clear() { *s = importState{} }

// clearImportState forgets picked paths and previews (Lock, Shutdown).
func (a *App) clearImportState() {
	a.mu.Lock()
	a.imp.clear()
	a.mu.Unlock()
}

// planHash is SHA-256 over a canonical encoding of the sorted name sets:
// per section a marker byte and a count, then each name length-prefixed, so
// no two different plans share an encoding.
func planHash(newNames, dupNames []string) [32]byte {
	h := sha256.New()
	var n [8]byte
	section := func(marker byte, names []string) {
		h.Write([]byte{marker})
		binary.BigEndian.PutUint64(n[:], uint64(len(names)))
		h.Write(n[:])
		for _, s := range names {
			binary.BigEndian.PutUint64(n[:], uint64(len(s)))
			h.Write(n[:])
			h.Write([]byte(s))
		}
	}
	section('N', newNames)
	section('D', dupNames)
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// PickEnvFile asks the user for a .env file. "" means the user cancelled.
// Only paths returned here can be previewed or applied.
func (a *App) PickEnvFile() (string, error) {
	if a.deps.Dialogs == nil {
		return "", unsupported()
	}
	p, err := a.deps.Dialogs.OpenFile("Import .env")
	if err != nil {
		return "", newAPIError(CodeInternal, "could not open the file dialog")
	}
	if p == "" {
		return "", nil
	}
	a.mu.Lock()
	a.imp.pick(p)
	a.mu.Unlock()
	return p, nil
}

func (a *App) isPicked(p string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return p != "" && slices.Contains(a.imp.picked, p)
}

func errNotPicked() error {
	return newAPIError(CodeInvalidValue, "choose the file with the file picker first")
}

// readEnvFile opens and parses path as dotenv. Errors are value-free, fixed
// messages. The importexport errors only carry the line number as text, so
// the line is not surfaced.
func readEnvFile(path string) ([]importexport.Entry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, newAPIError(CodeNotFound, "the file could not be read")
	}
	if !st.Mode().IsRegular() {
		return nil, newAPIError(CodeInvalidValue, "the path is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, newAPIError(CodeNotFound, "the file could not be read")
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, newAPIError(CodeInvalidValue, "the path is not a regular file")
	}
	entries, err := importexport.Parse(importexport.FormatDotenv, f)
	switch {
	case err == nil:
		return entries, nil
	case errors.Is(err, domain.ErrUnsupportedValue): // dotenv: only the 8 MiB cap
		return nil, newAPIError(CodeInvalidValue, "the file is too large")
	case errors.Is(err, domain.ErrUnsupportedFormat), errors.Is(err, domain.ErrInvalidName):
		return nil, newAPIError(CodeInvalidValue, "the file contains an invalid entry")
	}
	return nil, newAPIError(CodeNotFound, "the file could not be read")
}

func clearEntries(es []importexport.Entry) {
	for i := range es {
		clear(es[i].Value)
	}
}

// planFile reads path and plans it against envID inside u. It returns the
// plan, its sorted names and their hash. The entry values are cleared before
// returning (the plan keeps its own copies).
func planFile(ctx context.Context, u *app.Unlocked, envID, path string) (app.ImportPlan, []string, []string, [32]byte, error) {
	entries, err := readEnvFile(path)
	if err != nil {
		return app.ImportPlan{}, nil, nil, [32]byte{}, err
	}
	defer clearEntries(entries)
	plan, err := u.PlanImport(ctx, eid(envID), entries)
	if err != nil {
		return app.ImportPlan{}, nil, nil, [32]byte{}, err
	}
	newN, dupN := names(plan.New), names(plan.Duplicate)
	return plan, newN, dupN, planHash(newN, dupN), nil
}

func names(ns []domain.SecretName) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = string(n)
	}
	slices.Sort(out)
	return out
}

// PreviewImport lists which names of the picked file are new and which
// already exist in envID. Names only.
func (a *App) PreviewImport(envID, path string) (ImportPreview, error) {
	if !a.isPicked(path) {
		return ImportPreview{}, errNotPicked()
	}
	var out ImportPreview
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		_, newN, dupN, h, err := planFile(ctx, u, envID, path)
		if err != nil {
			return err
		}
		a.mu.Lock()
		a.imp.put(previewKey{envID, path}, h)
		a.mu.Unlock()
		out = ImportPreview{New: newN, Duplicate: dupN}
		return nil
	})
	if err != nil {
		return ImportPreview{}, err
	}
	return out, nil
}

// ApplyImport imports the picked file into envID with strategy skip or
// overwrite, but only if a fresh read of the file and the vault still yields
// exactly the previewed name sets; otherwise it returns conflict and imports
// nothing.
func (a *App) ApplyImport(envID, path, strategy string) error {
	st := app.Strategy(strategy)
	if st != app.StrategySkip && st != app.StrategyOverwrite {
		return newAPIError(CodeInvalidValue, "strategy must be skip or overwrite")
	}
	if !a.isPicked(path) {
		return errNotPicked()
	}
	k := previewKey{envID, path}
	a.mu.Lock()
	want, ok := a.imp.previews[k]
	a.mu.Unlock()
	if !ok {
		return newAPIError(CodeConflict, "preview the import first")
	}
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		plan, _, _, h, err := planFile(ctx, u, envID, path)
		if err != nil {
			return err
		}
		if h != want {
			a.mu.Lock()
			a.imp.drop(k)
			a.mu.Unlock()
			return newAPIError(CodeConflict, "the file or the environment changed; preview again")
		}
		return u.ApplyImport(ctx, eid(envID), plan, st)
	})
	if err == nil {
		a.mu.Lock()
		a.imp.drop(k)
		a.mu.Unlock()
	}
	return err
}
