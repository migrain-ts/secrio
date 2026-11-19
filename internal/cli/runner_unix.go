//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// statusCode maps a finished process to its exit code; a child killed by
// signal N yields 128+N.
func statusCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return exitSignalShift + int(ws.Signal())
	}
	return ps.ExitCode()
}
