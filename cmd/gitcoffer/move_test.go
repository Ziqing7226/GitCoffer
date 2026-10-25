package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

func makeVaultAt(t *testing.T, dir, pass string) vault.Meta {
	t.Helper()
	s, err := vault.Create(dir, pass, crypto.DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	return s.Meta()
}

func TestMoveSameFilesystemRenames(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "v.coffer")
	meta := makeVaultAt(t, src, "pass123")
	dst := filepath.Join(base, "sub", "v2.coffer") // parent dir created on demand

	out := captureStdout(t, func() {
		if err := moveCmd([]string{src, dst}); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present after same-FS move: %v", err)
	}
	got, err := vault.ReadMeta(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != meta.ID {
		t.Fatalf("moved vault id = %s, want %s", got.ID, meta.ID)
	}
	for _, want := range []string{
		`git remote set-url origin "coffer::` + dst + `"`,
		"Vault " + meta.ID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("move report lacks %q:\n%s", want, out)
		}
	}
}

func TestMoveRefusals(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "v.coffer")
	makeVaultAt(t, src, "pass123")

	notVault := t.TempDir()
	occupied := filepath.Join(base, "occupied")
	if err := os.MkdirAll(filepath.Join(occupied, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(base, "existing.coffer")
	makeVaultAt(t, existing, "pass123")

	cases := []struct {
		name      string
		args      []string
		wantInErr string
	}{
		{"not a vault", []string{notVault, filepath.Join(base, "a")}, "not a vault"},
		{"target not empty", []string{src, occupied}, "not empty"},
		{"target is a vault", []string{src, existing}, "already a vault"},
		{"target inside source", []string{src, filepath.Join(src, "sub")}, "inside"},
		{"same path", []string{src, src}, "same path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := moveCmd(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantInErr) {
				t.Fatalf("moveCmd(%v) = %v, want error containing %q", tc.args, err, tc.wantInErr)
			}
		})
	}
	// Nothing was moved by the refused cases.
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source vanished after a refused move")
	}
}

func TestMoveRefusesActiveLock(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "v.coffer")
	makeVaultAt(t, src, "pass123")
	host, _ := os.Hostname()
	lock := fmt.Sprintf("host=%s\npid=%d\nstarted=%s\n",
		host, os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(src, "vault.lock"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	err := moveCmd([]string{src, filepath.Join(base, "dst.coffer")})
	if err == nil || !strings.Contains(err.Error(), "being written") {
		t.Fatalf("move under an active lock = %v, want refusal", err)
	}
}

func TestLockHeldThreeStates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	makeVaultAt(t, dir, "pass123")

	if vault.LockHeld(dir) {
		t.Fatal("LockHeld true with no lock file")
	}
	host, _ := os.Hostname()
	live := fmt.Sprintf("host=%s\npid=%d\nstarted=%s\n",
		host, os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	os.WriteFile(filepath.Join(dir, "vault.lock"), []byte(live), 0o600)
	if !vault.LockHeld(dir) {
		t.Fatal("LockHeld false with a live lock")
	}
	stale := fmt.Sprintf("host=%s\npid=%d\nstarted=%s\n",
		host, os.Getpid(), time.Now().UTC().Add(-20*time.Minute).Format(time.RFC3339Nano))
	os.WriteFile(filepath.Join(dir, "vault.lock"), []byte(stale), 0o600)
	if vault.LockHeld(dir) {
		t.Fatal("LockHeld true with a stale lock")
	}
}

func TestCopyVaultTree(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "tree")
	if err := os.MkdirAll(filepath.Join(src, "obj"), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "vault.meta"), []byte("meta"), 0o600)
	os.WriteFile(filepath.Join(src, "obj", "abc"), []byte("payload-abc"), 0o600)
	dst := filepath.Join(base, "copy")
	if err := copyVaultTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"vault.meta": "meta",
		"obj/abc":    "payload-abc",
	} {
		data, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil || string(data) != want {
			t.Errorf("copied %s = %q, %v; want %q", rel, data, err, want)
		}
	}

	// A planted symlink must abort the copy rather than read through it.
	symlinked := filepath.Join(base, "symlinked")
	os.MkdirAll(symlinked, 0o700)
	os.WriteFile(filepath.Join(symlinked, "vault.meta"), []byte("m"), 0o600)
	outside := filepath.Join(base, "outside-secret")
	os.WriteFile(outside, []byte("secret"), 0o600)
	if err := os.Symlink(outside, filepath.Join(symlinked, "obj")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := copyVaultTree(symlinked, filepath.Join(base, "copy2"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("copy through a symlink = %v, want refusal", err)
	}
}

func TestCopyVaultVerified(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "v.coffer")
	makeVaultAt(t, src, "pass123")
	dst := filepath.Join(base, "copy.coffer")

	if err := copyVaultVerified(src, dst, "wrong-pass"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("failed verify left a target behind")
	}
	if err := copyVaultVerified(src, dst, "pass123"); err != nil {
		t.Fatalf("copyVaultVerified: %v", err)
	}
	if _, err := vault.Open(dst, "pass123"); err != nil {
		t.Fatalf("copied vault does not open: %v", err)
	}
}

func TestKeyfileSlotNotes(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "v.coffer")
	meta := makeVaultAt(t, src, "pass123")
	// Extra slots covering every shape: an absolute path inside the
	// vault (breaks: stale path), a relative path pointing outside the
	// vault (breaks: its base changed), a relative path inside the vault
	// (travels along, keeps working), and an absolute path elsewhere
	// (untouched). Only the fields the notes read are needed.
	meta.Slots = append(meta.Slots,
		crypto.Slot{ID: 1, Input: "passphrase+keyfile", Keyfile: filepath.Join(src, "key.bin")},
		crypto.Slot{ID: 2, Input: "passphrase+keyfile", Keyfile: "../outside.key"},
		crypto.Slot{ID: 3, Input: "passphrase+keyfile", Keyfile: "keys/main.key"},
		crypto.Slot{ID: 4, Input: "passphrase+keyfile", Keyfile: filepath.Join(base, "elsewhere.key")},
	)
	notes := keyfileSlotNotes(meta, src)
	if len(notes) != 2 {
		t.Fatalf("keyfileSlotNotes = %v, want 2 notes", notes)
	}
	if !strings.Contains(notes[0], "slot 1") || !strings.Contains(notes[0], "inside the vault") {
		t.Errorf("inside-vault note wrong: %s", notes[0])
	}
	if !strings.Contains(notes[1], "slot 2") || !strings.Contains(notes[1], "relative path") {
		t.Errorf("relative-outside note wrong: %s", notes[1])
	}
}

func TestSyncyFilesystem(t *testing.T) {
	for name, want := range map[string]bool{
		"cifs": true, "smbfs": true, "nfs4": true, "prjfs": true,
		"fuse.rclone": true, "davfs2": true,
		"ext4": false, "ntfs": false, "exfat": false, "vfat": false, "apfs": false,
	} {
		if got := syncyFilesystem(name); got != want {
			t.Errorf("syncyFilesystem(%q) = %v, want %v", name, got, want)
		}
	}
}
