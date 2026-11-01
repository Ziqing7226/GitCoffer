//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// filesystemName reports the filesystem holding path, best effort.
// statfs yields magic numbers only — every FUSE/network filesystem
// reports "unknown" — so /proc/self/mountinfo is consulted first: it
// names the filesystem type (fuse.sshfs, nfs4, cifs, …) for the mount
// point actually covering the path, which is what the doctor probe and
// the move command's sync-layer caution need.
func filesystemName(path string) (string, bool) {
	if name, ok := fstypeFromMountinfo(path); ok {
		return name, true
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return "", false
	}
	return linuxFsName(uint64(st.Type)), true
}

// fstypeFromMountinfo finds the deepest mount in /proc/self/mountinfo
// that contains path and returns its filesystem type.
func fstypeFromMountinfo(path string) (string, bool) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", false
	}
	// The mountinfo match is lexical, so resolve symlinks first: a vault
	// reached through one (e.g. /media/usb -> /run/media/user/USB) would
	// otherwise match only the root mount and report the root's type.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	bestMount, bestType := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		mount, fstype, ok := parseMountinfoLine(line)
		if !ok || !withinMount(abs, mount) {
			continue
		}
		// Deepest mount wins; >= keeps the last of equal-depth mounts,
		// which is deterministic for a fixed file.
		if len(mount) >= len(bestMount) {
			bestMount, bestType = mount, fstype
		}
	}
	return bestType, bestMount != ""
}

// parseMountinfoLine reads one /proc/self/mountinfo line:
// "id parent maj:min root mount-point options… - fstype source super…"
func parseMountinfoLine(line string) (mountPoint, fstype string, ok bool) {
	halves := strings.SplitN(line, " - ", 2)
	if len(halves) != 2 {
		return "", "", false
	}
	fields := strings.Fields(halves[0])
	if len(fields) < 5 {
		return "", "", false
	}
	tf := strings.Fields(halves[1])
	if len(tf) < 1 {
		return "", "", false
	}
	return unescapeMountPath(fields[4]), tf[0], true
}

// withinMount reports whether path lies at or under mount ("/" always
// contains; otherwise a component-boundary prefix).
func withinMount(path, mount string) bool {
	if mount == "/" {
		return true
	}
	return path == mount || strings.HasPrefix(path, mount+"/")
}

// unescapeMountPath decodes the octal escapes mountinfo uses for
// whitespace and backslashes in mount points.
func unescapeMountPath(p string) string {
	if !strings.Contains(p, "\\") {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' && i+3 < len(p) {
			switch p[i+1 : i+4] {
			case "040":
				b.WriteByte(' ')
				i += 3
				continue
			case "011":
				b.WriteByte('\t')
				i += 3
				continue
			case "134":
				b.WriteByte('\\')
				i += 3
				continue
			case "012":
				b.WriteByte('\n')
				i += 3
				continue
			}
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

func linuxFsName(magic uint64) string {
	switch magic {
	case 0x4d44:
		return "vfat"
	case 0x2011bab0:
		return "exfat"
	case 0xef53:
		return "ext4"
	case 0x9123683e:
		return "btrfs"
	case 0x01021994:
		return "tmpfs"
	case 0x58465342:
		return "xfs"
	}
	return "unknown"
}
