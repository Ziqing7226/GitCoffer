package main

// gitcoffer move relocates a vault directory and nothing more. Host↔vault
// relocations are the documented project-migration steps (plain git); a
// cloud target is an ordinary mounted path. During the move the source is
// never modified, so the worst outcome is a cleaned-up partial target.
//
// Same-filesystem moves rename atomically. Cross-filesystem moves copy
// under the source's writer lock (concurrent pushes cannot interleave
// with the copy), then verify the copy — open and full fsck — before
// reporting success, and only then ask whether to drop the source.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ziqing7226/GitCoffer/internal/vault"
	"golang.org/x/term"
)

func moveCmd(args []string) error {
	fs := flag.NewFlagSet("move", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gitcoffer move <vault-directory> <new-path>")
		fmt.Fprintln(os.Stderr, usageHint("move"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		os.Exit(2)
	}
	srcRaw, dstRaw := fs.Arg(0), fs.Arg(1)
	src, err := filepath.Abs(srcRaw)
	if err != nil {
		return err
	}
	dst, err := filepath.Abs(dstRaw)
	if err != nil {
		return err
	}
	src, dst = filepath.Clean(src), filepath.Clean(dst)

	// Source sanity: a vault we can read the header of, not mid-write.
	meta, err := vault.ReadMeta(src)
	if err != nil {
		if errors.Is(err, vault.ErrNotVault) {
			return fmt.Errorf("source is not a vault (%v) — move takes an existing vault directory; to copy a project off a host use the migration steps in the user guide", err)
		}
		return fmt.Errorf("source vault is not readable: %v — repair it (gitcoffer fsck) before moving", err)
	}
	if vault.LockHeld(src) {
		return fmt.Errorf("the vault is being written to right now (another coffer operation holds the lock) — wait for it to finish, then move again")
	}
	if src == dst {
		return errors.New("source and target are the same path")
	}
	if insideSource(src, dst) {
		return fmt.Errorf("target %s lies inside the vault being moved — choose a target outside %s", dst, src)
	}

	// Target sanity: a fresh name, an empty directory, or nothing yet.
	if info, err := os.Lstat(dst); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("target %s exists and is a file", dst)
		}
		entries, rerr := os.ReadDir(dst)
		if rerr != nil {
			return rerr
		}
		if len(entries) > 0 {
			if _, merr := vault.ReadMeta(dst); merr == nil {
				return fmt.Errorf("target %s is already a vault — refusing to overwrite it; move to a fresh path instead", dst)
			}
			return fmt.Errorf("target %s exists and is not empty — refusing; move to a fresh path (an empty directory is fine)", dst)
		}
		// An empty directory is acceptable; remove it so the rename below
		// finds a free name (Windows cannot rename over a directory).
		if err := os.Remove(dst); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	keyfileNotes := keyfileSlotNotes(meta, src)

	// Same-filesystem: one atomic rename, no passphrase needed. A
	// cross-filesystem rename fails with the platform's not-same-device
	// error and falls through to the copy path.
	if err := os.Rename(src, dst); err == nil {
		moveReport(meta, src, dst, keyfileNotes, false)
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("moving the vault failed: %v — the vault is untouched at %s", err, src)
	}

	// Cross-filesystem: verify the source opens, freeze writers, copy,
	// then verify the copy end to end before claiming success.
	pass, err := promptPassword("Passphrase")
	if err != nil {
		return err
	}
	if err := copyVaultVerified(src, dst, pass); err != nil {
		return fmt.Errorf("%v — the source is untouched and nothing was left at the target", err)
	}

	moveReport(meta, src, dst, keyfileNotes, true)

	// The copy is verified. Removing the source is the user's call;
	// declining (or a non-interactive run) keeps it as the natural backup.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("Remove the source vault at %s now that the copy is verified? [y/N]: ", src)
		answer, _ := readStdinLine()
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			if err := os.RemoveAll(src); err != nil {
				return fmt.Errorf("removing the source failed (%v) — remove %s by hand when ready", err, src)
			}
			fmt.Println("Source removed.")
		default:
			fmt.Printf("Source kept at %s — it is the natural backup; remove it by hand once the new location has proven itself.\n", src)
		}
	} else {
		fmt.Printf("Source kept at %s (non-interactive run) — it is the natural backup; remove it by hand once the new location has proven itself.\n", src)
	}
	return nil
}

// copyVaultVerified copies the vault at src to dst under the source's
// writer lock and verifies the copy end to end (it opens, and fsck finds
// no errors). On any failure the partial target is removed — the worst
// outcome of a move — and the source is left untouched.
func copyVaultVerified(src, dst, pass string) error {
	s, err := vault.Open(src, pass)
	if err != nil {
		return err
	}
	release, err := s.AcquireLock()
	if err != nil {
		return err
	}
	defer release()
	if err := copyVaultTree(src, dst); err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("copying the vault failed: %v", err)
	}
	if _, err := vault.Open(dst, pass); err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("the copied vault does not open: %v", err)
	}
	findings, err := vault.Fsck(dst, pass)
	if err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("verifying the copied vault failed: %v", err)
	}
	for _, f := range findings {
		if f.Err {
			os.RemoveAll(dst)
			return fmt.Errorf("verification failed: fsck reported errors — the copied vault was removed")
		}
	}
	return nil
}

// moveReport prints what every clone needs next, plus the caveats the
// move itself cannot fix: key-file slots whose registered path no longer
// resolves, and targets on sync-client or online layers.
func moveReport(meta vault.Meta, src, dst string, keyfileNotes []string, copied bool) {
	verb := "moved"
	if copied {
		verb = "copied and verified"
	}
	fmt.Printf("Vault %s: %s\n  from: %s\n  to:   %s\n", meta.ID, verb, src, dst)
	fmt.Println("Update every clone's remote URL, e.g.:")
	fmt.Printf("  git remote set-url origin \"coffer::%s\"\n", dst)
	for _, note := range keyfileNotes {
		fmt.Println(note)
	}
	if fsName, known := filesystemName(dst); known && syncyFilesystem(fsName) {
		fmt.Printf("  [warn] the target sits on %s, an online or synced layer — the vault stays encrypted, but a sync client can hold half-updated copies after an interruption; run gitcoffer fsck if a sync was interrupted mid-write\n", strings.ToLower(fsName))
	}
}

// keyfileSlotNotes flags key-file slots whose recorded path this move
// invalidates. Three shapes: an absolute path inside the moved tree
// travels along (content recognition finds it next to — or inside — the
// moved vault, but the slot still names the old location); a relative
// path that pointed outside the vault loses its base; a relative path
// inside the vault travels and keeps resolving. Absolute paths elsewhere
// are untouched.
func keyfileSlotNotes(meta vault.Meta, src string) []string {
	var notes []string
	for _, sl := range meta.Slots {
		if sl.Input != "passphrase+keyfile" || sl.Keyfile == "" {
			continue
		}
		registered := sl.Keyfile
		if !filepath.IsAbs(registered) {
			resolved := filepath.Clean(filepath.Join(src, registered))
			if insideSource(src, resolved) {
				continue // travels with the vault; resolves the same way at the target
			}
			notes = append(notes, fmt.Sprintf(
				"  [warn] slot %d's key file is recorded as the relative path %s, which pointed outside the vault — the vault directory was its base and just changed; the slot still opens when the key file is recognized by content next to the moved vault, or when it is re-registered via gitcoffer key add",
				sl.ID, sanitizePath(registered)))
			continue
		}
		if insideSource(src, filepath.Clean(registered)) {
			notes = append(notes, fmt.Sprintf(
				"  [warn] slot %d's key file lives inside the vault and is recorded at its old path (%s) — the file moved along and content recognition finds it at the new location; re-register it with gitcoffer key add to record the new path",
				sl.ID, sanitizePath(registered)))
		}
	}
	return notes
}

// copyVaultTree copies the vault at src to dst recursively. The source is
// never modified. The writer lock file is excluded on purpose: it belongs
// to this very operation, and a copied lock would look fresh at the
// target, blocking the next writer for the staleness window. Planted
// symlinks and other non-regular entries abort the copy — following a
// symlink out of the vault would read files that are nobody's to move.
func copyVaultTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			return os.MkdirAll(dst, info.Mode().Perm())
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.Mkdir(target, info.Mode().Perm())
		case d.Type()&os.ModeSymlink != 0:
			return fmt.Errorf("unexpected symlink at %s — refusing to copy through it", sanitizePath(rel))
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unexpected entry %s (not a regular file) — refusing to copy it", sanitizePath(rel))
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// insideSource reports whether dst lies inside src (or equals it).
func insideSource(src, dst string) bool {
	rel, err := filepath.Rel(src, dst)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// syncyFilesystem reports whether a filesystem name belongs to an online
// or sync-client layer — network filesystems, cloud-drive projections,
// and sync-FUSE mounts. Vaults work there (the untrusted-storage niche),
// but the layer decides when files are consistent, which deserves a word.
func syncyFilesystem(name string) bool {
	switch strings.ToLower(name) {
	case "cifs", "smb", "smb2", "smbfs", "ncpfs",
		"nfs", "nfs2", "nfs3", "nfs4", "nfsd",
		"afp", "afpfs",
		"prjfs", // Windows projected FS: OneDrive-style cloud folders
		"davfs2", "webdav":
		return true
	}
	return strings.Contains(strings.ToLower(name), "rclone")
}

// readStdinLine reads one line from stdin for interactive questions.
func readStdinLine() (string, error) {
	if stdinLines == nil {
		stdinLines = bufio.NewReader(os.Stdin)
	}
	line, err := stdinLines.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
