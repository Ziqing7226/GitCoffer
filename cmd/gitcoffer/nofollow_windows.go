//go:build windows

package main

import "os"

// openFileNoFollow opens path read-only for the move copy. On Windows,
// creating symlinks requires privileges and planted reparse points are
// not a realistic vector for a vault on removable media; the copy's
// post-open regular-file re-check (copyFile) is the defense here, and
// WalkDir's d.Type() classification refused reparse points before the
// open anyway.
func openFileNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
