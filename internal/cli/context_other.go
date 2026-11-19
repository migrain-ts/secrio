//go:build windows

package cli

import "os"

// ownerUID: Windows has no uid; the ownership check is skipped.
func ownerUID(os.FileInfo) (uint32, bool) { return 0, false }
