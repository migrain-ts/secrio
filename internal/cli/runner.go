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
// relayed to the child; when sigs is nil it subscribes to os.Interrupt and
// SIGTERM for the duration of each run.
func systemRunner(sigs <-chan os.Signal) RunFunc {
	return func(_ context.Context, name string, args, env []string, in io.Reader, out, errw io.Writer) (int, error) {
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
