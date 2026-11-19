//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// ownerUID returns the uid owning the file, if the platform reports it.
func ownerUID(fi os.FileInfo) (uint32, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}
