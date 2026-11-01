//go:build linux

package main

import "testing"

func TestParseMountinfoLine(t *testing.T) {
	cases := []struct {
		line       string
		mount, fst string
		ok         bool
	}{
		{
			"36 35 98:0 / /mnt/usb rw,noatime - vfat /dev/sdb1 rw",
			"/mnt/usb", "vfat", true,
		},
		{
			// Escaped space in the mount point (mountinfo \040).
			"40 35 8:1 / /mnt/my\\040drive rw - ext4 /dev/sda2 rw",
			"/mnt/my drive", "ext4", true,
		},
		{
			"1402 35 0:662 / /run/user/1000/doc rw,nosuid - fuse.portal /run/user/1000/doc rw",
			"/run/user/1000/doc", "fuse.portal", true,
		},
		{"garbage", "", "", false},
		{"a b c d", "", "", false},
		{"1 2 8:1 / /x rw - ", "", "", false},
	}
	for _, tc := range cases {
		mount, fst, ok := parseMountinfoLine(tc.line)
		if ok != tc.ok || mount != tc.mount || fst != tc.fst {
			t.Errorf("parseMountinfoLine(%q) = %q, %q, %v; want %q, %q, %v",
				tc.line, mount, fst, ok, tc.mount, tc.fst, tc.ok)
		}
	}
}

func TestWithinMount(t *testing.T) {
	if !withinMount("/a/b", "/") {
		t.Error("root must contain everything")
	}
	if !withinMount("/mnt/usb", "/mnt/usb") || !withinMount("/mnt/usb/v.coffer", "/mnt/usb") {
		t.Error("same or deeper path must be within")
	}
	if withinMount("/mnt/usb2/v", "/mnt/usb") {
		t.Error("sibling prefix must not match")
	}
}

// The mountinfo lookup must answer for a real path with a real
// filesystem name (not statfs's "unknown" for FUSE/network layers); the
// exact type depends on the host, so assert plausibility only.
func TestFilesystemNameUsesMountinfo(t *testing.T) {
	name, ok := filesystemName("/tmp")
	if !ok || name == "" || name == "unknown" {
		t.Fatalf("filesystemName(/tmp) = %q, %v; want a concrete type", name, ok)
	}
}
