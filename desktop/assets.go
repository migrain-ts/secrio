//go:build desktopapp

// Package desktopassets embeds the built frontend for the desktop shell. The
// build tag keeps untagged builds working when frontend/dist does not exist.
package desktopassets

import "embed"

// Assets is the Vite build output (frontend/dist).
//
//go:embed all:frontend/dist
var Assets embed.FS
