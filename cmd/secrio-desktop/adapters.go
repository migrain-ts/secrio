//go:build desktopapp

package main

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"secrio/internal/desktop"
)

// errNotStarted is returned when the shell is used before Wails' OnStartup.
// The Wails runtime calls log.Fatal on a nil context, so we never pass one.
var errNotStarted = errors.New("desktop shell not started")

// shellCtx holds the context Wails passes to OnStartup.
type shellCtx struct {
	p atomic.Pointer[context.Context]
}

func (s *shellCtx) set(ctx context.Context) { s.p.Store(&ctx) }

func (s *shellCtx) get() (context.Context, error) {
	if p := s.p.Load(); p != nil && *p != nil {
		return *p, nil
	}
	return nil, errNotStarted
}

// wailsClipboard implements desktop.Clipboard over the Wails runtime.
type wailsClipboard struct{ ctx *shellCtx }

func (c wailsClipboard) GetText() (string, error) {
	ctx, err := c.ctx.get()
	if err != nil {
		return "", fmt.Errorf("%w: %w", desktop.ErrClipboardUnreadable, err)
	}
	s, err := runtime.ClipboardGetText(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", desktop.ErrClipboardUnreadable, err)
	}
	return s, nil
}

func (c wailsClipboard) SetText(s string) error {
	ctx, err := c.ctx.get()
	if err != nil {
		return err
	}
	return runtime.ClipboardSetText(ctx, s)
}

// wailsDialogs implements desktop.Dialogs over the Wails runtime.
type wailsDialogs struct{ ctx *shellCtx }

// OpenFile shows a native open-file dialog; "" means the user cancelled.
func (d wailsDialogs) OpenFile(title string) (string, error) {
	ctx, err := d.ctx.get()
	if err != nil {
		return "", err
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:           title,
		Filters:         envFilters(),
		ShowHiddenFiles: true, // .env files are dot files, hidden by default on macOS
	})
}

// envFilters lists the dialog filters. On macOS Wails merges every pattern
// into one set of allowed file extensions (UTType per extension, no filter
// menu), so ".env", ".env.*" and "All files" cannot be expressed there: any
// file is allowed instead, and the facade validates the file it reads.
func envFilters() []runtime.FileFilter {
	if goruntime.GOOS == "darwin" {
		return nil
	}
	return []runtime.FileFilter{
		{DisplayName: "Env files (.env, *.env, .env.*)", Pattern: ".env;*.env;.env.*"},
		{DisplayName: "All files", Pattern: "*"},
	}
}
