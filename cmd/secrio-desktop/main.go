//go:build desktopapp

// Command secrio-desktop is the Wails desktop shell. Build it with
// scripts/desktop-build.sh (wails build -tags desktopapp), not go build.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	desktopassets "secrio/desktop"
	"secrio/internal/app"
	"secrio/internal/core"
	"secrio/internal/desktop"
)

// dataDir resolves the data directory with the CLI's rule (cli.RealDeps):
// SECRIO_HOME if set, otherwise the platform default.
func dataDir() (string, error) {
	if h := os.Getenv("SECRIO_HOME"); h != "" {
		return filepath.Clean(h), nil
	}
	return core.DefaultDataDir()
}

func main() {
	sc := &shellCtx{}
	a := desktop.New(desktop.Deps{
		NewService: func() (*app.Service, error) {
			dir, err := dataDir()
			if err != nil {
				return nil, err
			}
			return core.New(dir)
		},
		DataDir:   dataDir,
		Clipboard: wailsClipboard{ctx: sc},
		Dialogs:   wailsDialogs{ctx: sc},
	})

	err := wails.Run(&options.App{
		Title:            "Secrio",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: options.NewRGB(8, 9, 10),
		AssetServer:      &assetserver.Options{Assets: desktopassets.Assets},
		// Only the explicit allowlist in desktop.API is exposed to JS.
		Bind:      []any{desktop.NewAPI(a)},
		OnStartup: sc.set,
		// The only place the vault is locked and our clipboard copy cleared on
		// exit. Never add OnBeforeClose returning true: it blocks quitting.
		OnShutdown: func(context.Context) { a.Shutdown() },
		Mac: &mac.Options{
			Appearance: mac.NSAppearanceNameDarkAqua,
			About:      &mac.AboutInfo{Title: "Secrio", Message: "Local secrets manager"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
