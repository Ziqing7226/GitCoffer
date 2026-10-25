//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether a failed rename was refused because the
// two paths live on different filesystems — the signal to fall back from
// the atomic rename to the copy-and-verify path.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
