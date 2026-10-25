//go:build windows

package main

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether a failed rename was refused because the
// two paths live on different volumes — the signal to fall back from the
// atomic rename to the copy-and-verify path. Windows reports
// ERROR_NOT_SAME_DEVICE (winerror.h 17) for MoveFileEx across volumes
// without the copy-allowed flag; os.Rename never sets that flag.
func isCrossDevice(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.Errno(17)
	}
	return false
}
