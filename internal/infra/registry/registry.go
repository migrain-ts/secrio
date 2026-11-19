// Package registry persists the list of known vaults as a small JSON file.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"secrio/internal/app"
	"secrio/internal/domain"
)

const fileName = "vaults.json"

// Registry stores vault entries in <dir>/vaults.json (mode 0600).
//
// Read-modify-write cycles are serialized with an in-process mutex only.
// Known limitation: two processes writing concurrently can lose an update
// (last rename wins); a later task may add a lock file.
type Registry struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Registry { return &Registry{dir: dir} }

type entryJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (r *Registry) path() string { return filepath.Join(r.dir, fileName) }

func (r *Registry) List(_ context.Context) ([]app.VaultEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.read()
}

func (r *Registry) Add(_ context.Context, e app.VaultEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, err := r.read()
	if err != nil {
		return err
	}
	for _, x := range entries {
		if x.ID == e.ID {
			return fmt.Errorf("vault %q: %w", e.ID, domain.ErrDuplicate)
		}
	}
	return r.write(append(entries, e))
}

func (r *Registry) Remove(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, err := r.read()
	if err != nil {
		return err
	}
	for i, x := range entries {
		if x.ID == id {
			return r.write(append(entries[:i], entries[i+1:]...))
		}
	}
	return fmt.Errorf("vault %q: %w", id, domain.ErrNotFound)
}

func (r *Registry) read() ([]app.VaultEntry, error) {
	data, err := os.ReadFile(r.path())
	if errors.Is(err, os.ErrNotExist) {
		return []app.VaultEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var raw []entryJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("registry file malformed: %w", domain.ErrCorrupt)
	}
	out := make([]app.VaultEntry, len(raw))
	for i, x := range raw {
		out[i] = app.VaultEntry{ID: x.ID, Name: x.Name, Path: x.Path}
	}
	return out, nil
}

// write atomically replaces the file: temp file in the same dir, fsync, rename.
func (r *Registry) write(entries []app.VaultEntry) error {
	raw := make([]entryJSON, len(entries))
	for i, x := range entries {
		raw[i] = entryJSON{ID: x.ID, Name: x.Name, Path: x.Path}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registry: %w", err)
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	tmp, err := os.CreateTemp(r.dir, fileName+".tmp-*") // created with 0600
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write registry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync registry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), r.path()); err != nil {
		return fmt.Errorf("replace registry: %w", err)
	}
	return nil
}
