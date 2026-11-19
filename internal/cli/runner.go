package cli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Exit codes specific to `run` (besides the child's own code).
const (
	ExitRunFailure  = 125 // Secrio failed before launching the command
	ExitNotExec     = 126 // command found but could not be executed
	ExitNotFound    = 127 // command not found
	exitSignalShift = 128 // child killed by signal N exits 128+N
)

// RunFunc runs name with args and exactly env as its environment, wiring
// stdio, and returns the exit code to propagate. A non-nil error means the
// command could not be run (or waited for); the code still applies.
type RunFunc func(ctx context.Context, name string, args, env []string, in io.Reader, out, errw io.Writer) (exitCode int, err error)

// systemRunner returns the os/exec runner. Signals received on sigs are
// relayed to the child (os.Interrupt only when not interactive, see below);
// when sigs is nil it subscribes to os.Interrupt and SIGTERM for the
// duration of each run, so secrio survives them and waits for the child.
// interactive reports whether a terminal is attached; nil means not.
func systemRunner(sigs <-chan os.Signal, interactive func() bool) RunFunc {
	return func(_ context.Context, name string, args, env []string, in io.Reader, out, errw io.Writer) (int, error) {
		// On a terminal, Ctrl-C sends SIGINT to the whole foreground process
		// group, child included. Relaying it too would deliver it twice, which
		// many tools (e.g. Terraform) treat as "force quit now". Without a
		// terminal (e.g. `kill -INT <pid>`) the child only gets it from us.
		relayInterrupt := interactive == nil || !interactive()
		sigs := sigs // per call: never mutate the captured channel
		if sigs == nil {
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
			defer func() {
				signal.Stop(ch)
				close(ch)
			}()
			sigs = ch
		}
		cmd := exec.Command(name, args...)
		cmd.Env = env
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errw
		if err := cmd.Start(); err != nil {
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				return ExitNotFound, err
			}
			return ExitNotExec, err
		}

		stop, relayed := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(relayed)
			for {
				select {
				case s, ok := <-sigs:
					if !ok {
						return
					}
					if s == os.Interrupt && !relayInterrupt {
						continue // the terminal already delivered it
					}
					_ = cmd.Process.Signal(s) // e.g. SIGTERM cannot be relayed on Windows
				case <-stop:
					return
				}
			}
		}()
		err := cmd.Wait()
		close(stop)
		<-relayed

		var ee *exec.ExitError
		switch {
		case err == nil || errors.As(err, &ee):
			return statusCode(cmd.ProcessState), nil
		case cmd.ProcessState != nil && statusCode(cmd.ProcessState) != 0:
			return statusCode(cmd.ProcessState), err
		default:
			return ExitRunFailure, err // e.g. stdio copy failure
		}
	}
}
