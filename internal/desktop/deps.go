// Package desktop is the facade the Wails shell binds to. It talks to the
// Core only through app and domain types.
package desktop

import (
	"errors"
	"time"

	"secrio/internal/app"
)

// Clipboard is the text clipboard the desktop shell provides.
type Clipboard interface {
	GetText() (string, error)
	SetText(string) error
}

// Dialogs are the native dialogs the desktop shell provides.
type Dialogs interface {
	OpenFile(title string) (string, error)
}

// ErrClipboardUnreadable means the clipboard content cannot be read as text.
var ErrClipboardUnreadable = errors.New("clipboard is not readable")

// Session lengths used when unlocking a vault.
const (
	sessionTTL  = 12 * time.Hour
	sessionIdle = time.Hour
)

// Deps are the App's collaborators. NewService and DataDir are required;
// Clipboard and Dialogs may be nil until the shell provides them.
type Deps struct {
	NewService func() (*app.Service, error)
	DataDir    func() (string, error)
	Clipboard  Clipboard
	Dialogs    Dialogs
	AfterFunc  func(d time.Duration, f func()) (stop func() bool)
}

func defaultAfterFunc(d time.Duration, f func()) func() bool {
	t := time.AfterFunc(d, f)
	return t.Stop
}
