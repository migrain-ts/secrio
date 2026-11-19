//go:build windows

package cli

import "os"

// statusCode maps a finished process to its exit code.
func statusCode(ps *os.ProcessState) int { return ps.ExitCode() }
