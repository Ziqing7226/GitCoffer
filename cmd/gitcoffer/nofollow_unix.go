//go:build !windows

package main

import (
	"os"
	"syscall"
)

// openFileNoFollow opens path read-only, refusing to traverse a final
// symlink (the same planted-entry defense every vault read uses): the
// move copy's inputs come from the medium, and an entry swapped to a
// symlink between the walk's type check and this open must fail the
// copy, not read through to a file outside the vault.
func openFileNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}
