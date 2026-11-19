//go:build desktopapp

package main

import (
	"errors"
	"testing"

	"secrio/internal/desktop"
)

// Before OnStartup the adapters must fail, never reach the Wails runtime
// (which calls log.Fatal on a nil context).
func TestAdaptersBeforeStartup(t *testing.T) {
	sc := &shellCtx{}
	if _, err := (wailsClipboard{ctx: sc}).GetText(); !errors.Is(err, desktop.ErrClipboardUnreadable) {
		t.Fatalf("GetText: %v", err)
	}
	if err := (wailsClipboard{ctx: sc}).SetText("x"); !errors.Is(err, errNotStarted) {
		t.Fatalf("SetText: %v", err)
	}
	if _, err := (wailsDialogs{ctx: sc}).OpenFile("t"); !errors.Is(err, errNotStarted) {
		t.Fatalf("OpenFile: %v", err)
	}
	sc.set(nil)
	if _, err := sc.get(); !errors.Is(err, errNotStarted) {
		t.Fatalf("nil ctx: %v", err)
	}
}
