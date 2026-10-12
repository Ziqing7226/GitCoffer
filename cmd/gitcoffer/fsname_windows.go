//go:build windows

package main

import "golang.org/x/sys/windows"

// filesystemName reports the filesystem holding path, best effort.
// GetVolumePathName resolves the true volume root (drive letter, folder
// mount, or UNC share), so the probe answers for the volume the vault
// actually lives on rather than an unrelated path prefix.
func filesystemName(path string) (string, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return "", false
	}
	var fs [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumeInformation(&root[0], nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); err != nil {
		return "", false
	}
	name := windows.UTF16ToString(fs[:])
	return name, name != ""
}
