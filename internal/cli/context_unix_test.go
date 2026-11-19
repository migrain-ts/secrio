//go:build !windows

package cli

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO named .secrio.toml must be skipped, not read (a read would block
// forever); the search continues upward.
func TestLoadContextFIFOSkipped(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := syscall.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(sub, contextFile), 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	writeFile(t, filepath.Join(root, ".secrio.toml"), "vault = \"up\"\n")

	type res struct {
		c   Context
		err error
	}
	done := make(chan res, 1)
	go func() {
		c, err := LoadContext(ctxDeps(sub, nil), Flags{})
		done <- res{c, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || r.c.Vault != "up" {
			t.Fatalf("got %+v, %v", r.c, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoadContext blocks on a FIFO named .secrio.toml")
	}
}
