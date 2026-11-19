package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/term"

	"secrio/internal/app"
	"secrio/internal/core"
)

// Deps injects every side effect the commands need.
type Deps struct {
	In           io.Reader
	Out, Err     io.Writer
	Getenv       func(string) string
	Environ      func() []string
	Getwd        func() (string, error)
	StdinTTY     func() bool
	StdoutTTY    func() bool
	ReadPassword func(prompt string) ([]byte, error) // masked, from the terminal
	DataDir      func() (string, error)
	NewService   func() (*app.Service, error) // wraps core.New(dataDir)
	Run          RunFunc                      // runs `run`'s child process
}

// RealDeps wires the process environment.
func RealDeps() Deps {
	dataDir := func() (string, error) {
		if h := os.Getenv("SECRIO_HOME"); h != "" {
			return filepath.Clean(h), nil
		}
		return core.DefaultDataDir()
	}
	return Deps{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		Getenv:    os.Getenv,
		Environ:   os.Environ,
		Getwd:     os.Getwd,
		StdinTTY:  func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		StdoutTTY: func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },
		ReadPassword: func(prompt string) ([]byte, error) {
			fmt.Fprint(os.Stderr, prompt)
			pw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return pw, err
		},
		DataDir: dataDir,
		NewService: func() (*app.Service, error) {
			dir, err := dataDir()
			if err != nil {
				return nil, err
			}
			return core.New(dir)
		},
		Run: systemRunner(nil, func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) || term.IsTerminal(int(os.Stdout.Fd())) || term.IsTerminal(int(os.Stderr.Fd()))
		}),
	}
}
