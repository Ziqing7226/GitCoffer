package vault

// Key-slot management (spec §3): opening assembles each slot's KDF input
// (passphrase, or passphrase plus key-file bytes for second-factor slots),
// and key rotation rewrites only vault.meta — the DEK and every object
// file stay untouched.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
)

// slotSecret assembles the KDF input for one slot. A relative key-file
// path resolves against the vault directory — never the caller's working
// directory, which for the remote helper is the pushing repository.
func slotSecret(dir string, slot crypto.Slot, passphrase string) (string, error) {
	switch slot.Input {
	case "", crypto.InputPassphrase:
		return passphrase, nil
	case crypto.InputPassphraseKeyfile:
		if slot.Keyfile == "" {
			return "", fmt.Errorf("slot %d requires a key file but none is configured", slot.ID)
		}
		path := slot.Keyfile
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		// The path comes from plaintext vault.meta an attacker may have
		// rewritten: refuse non-regular files (a device or FIFO would
		// read forever) and bound the size.
		data, err := readLimited(path, 1<<20)
		if err != nil && slot.KeyfileID != "" {
			// The recorded path is gone — a move between machines or
			// drive letters. Recognize the key file by content.
			if found, ferr := FindKeyfileByID(dir, slot.KeyfileID); ferr == nil {
				if data, err = readLimited(found, 1<<20); err == nil {
					path = found
				}
			}
		}
		if err != nil {
			return "", fmt.Errorf("key file for slot %d: %v", slot.ID, sanitizePath(err.Error()))
		}
		if slot.KeyfileID != "" && crypto.Hash(data) != slot.KeyfileID {
			// The file at the recorded path changed since the slot was
			// made — not the key file this slot was sealed with.
			return "", fmt.Errorf("key file for slot %d: %s no longer matches the key file recorded for this slot (content changed)", slot.ID, sanitizePath(path))
		}
		return passphrase + string(data), nil
	default:
		// Unknown input type: a future format this build cannot use.
		return "", fmt.Errorf("slot %d: unsupported input %q", slot.ID, slot.Input)
	}
}

// sanitizePath strips control characters (including terminal escape
// sequences) from a path that originated in vault.meta before it is
// printed or embedded in an error hint.
func sanitizePath(p string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, p)
}

// refreshMeta re-reads vault.meta into the store. Meta operations call
// it under the writer lock: the lock serializes, but the in-memory meta
// is the snapshot from Open — without the refresh, a concurrent key
// operation that landed in between would be silently overwritten by our
// stale copy (a rekey reported as successful could be undone).
func (s *Store) refreshMeta() error {
	meta, err := ReadMeta(s.dir)
	if err != nil {
		return err
	}
	s.meta = meta
	return nil
}

// rewriteMeta atomically replaces vault.meta (spec §6, pattern 3). The
// take-over guard mirrors commit's: a key operation whose Argon2id work
// outlives the staleness window must not clobber whatever the thief
// committed.
func (s *Store) rewriteMeta() error {
	if s.lockGuard != "" {
		if err := s.verifyLockHeld(); err != nil {
			return fmt.Errorf("key operation aborted: %w", err)
		}
	}
	data, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.dir, metaName, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// OpenedSlotID returns the id of the key slot the current passphrase
// authenticated against.
func (s *Store) OpenedSlotID() int { return s.openedSlotID }

// AddSlot seals the vault's DEK under an additional passphrase, optionally
// combined with a key file (second factor). A keyfile path that does not
// yet exist is created with fresh random bytes, mode 0600, and its
// absolute path is recorded in the slot; the user must back it up
// separately from the vault. Returns the new slot id. Meta rewrites hold
// the writer lock so concurrent key operations refuse rather than race.
func (s *Store) AddSlot(newPass, keyfile string) (int, error) {
	return s.AddSlotParams(newPass, keyfile, crypto.DefaultParams())
}

// AddSlotParams is AddSlot with caller-chosen Argon2id parameters — the
// per-slot KDF cost selection of the 1.1.0 plan: each derivation is the
// price of every offline guess against the slot, forever.
func (s *Store) AddSlotParams(newPass, keyfile string, params crypto.Argon2Params) (int, error) {
	release, err := s.AcquireLock()
	if err != nil {
		return 0, err
	}
	defer release()
	if err := s.refreshMeta(); err != nil {
		return 0, err
	}
	if len(s.meta.Slots) >= maxKeySlots {
		return 0, fmt.Errorf("the vault already has the maximum of %d key slots", maxKeySlots)
	}
	id := 0
	for _, sl := range s.meta.Slots {
		if sl.ID >= id {
			id = sl.ID + 1
		}
	}
	secret := newPass
	var created *crypto.Slot
	if keyfile != "" {
		keyfile, err = ensureKeyfile(s.dir, keyfile)
		if err != nil {
			return 0, err
		}
		data, err := readLimited(keyfile, 1<<20)
		if err != nil {
			return 0, err
		}
		secret = newPass + string(data)
		created, err = crypto.SealSlot(id, secret, s.dek, s.meta.ID, params)
		if err == nil {
			created.Input = crypto.InputPassphraseKeyfile
			created.Keyfile = keyfile
			created.KeyfileID = crypto.Hash(data)
		}
	} else {
		created, err = crypto.SealSlot(id, secret, s.dek, s.meta.ID, params)
	}
	if err != nil {
		return 0, err
	}
	s.meta.Slots = append(s.meta.Slots, *created)
	if err := s.rewriteMeta(); err != nil {
		return 0, err
	}
	return id, nil
}

// findKeyfileByID looks for a file whose SHA-256 matches id, searching
// the vault's parent directory and then the vault directory itself —
// shallow and bounded: the places a vault-plus-key-file set travels
// together to, never a filesystem-wide scan. The parent comes first
// because a key file inside the vault is refused at creation (a moved-in
// copy is still accepted when found).
func FindKeyfileByID(vaultDir, id string) (string, error) {
	for _, dir := range []string{filepath.Dir(vaultDir), vaultDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names) // deterministic across directory orders
		for i, name := range names {
			if i >= maxKeyfileScan {
				break
			}
			path := filepath.Join(dir, name)
			info, ierr := os.Lstat(path)
			if ierr != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 || info.Size() < minKeyfileSize {
				continue
			}
			data, rerr := readLimited(path, 1<<20)
			if rerr != nil {
				continue
			}
			if crypto.Hash(data) == id {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("no file matching the recorded key file was found next to the vault")
}

// maxKeyfileScan bounds the content search: a drive root can hold more
// files than any recognition should read.
const maxKeyfileScan = 1000

// minKeyfileSize is the smallest existing file accepted as a key file: an
// empty or tiny file would make the "second factor" vacuous (the KDF input
// would degenerate to the passphrase alone) without any warning.
const minKeyfileSize = 16

// ensureKeyfile makes the key file exist: an existing file is used as-is
// (users may point at a file they manage), provided it carries real
// entropy; a missing one is created with 32 random bytes. The returned
// path is absolute.
// resolveExistingPath resolves symlinks in the longest existing prefix
// of p and rejoins the remainder. Used where a path may not exist yet
// (a key file being created) but must still be compared canonically.
func resolveExistingPath(p string) string {
	p = filepath.Clean(p)
	suffix := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, suffix)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, suffix)
		}
		suffix = filepath.Join(filepath.Base(p), suffix)
		p = parent
	}
}

func ensureKeyfile(vaultDir, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// A key file inside the vault directory would sit next to the
	// ciphertext it protects — the medium thief would hold the "second
	// factor" too. Refuse at creation. Containment is resolved against
	// the ABSOLUTE vault dir (a relative vault dir would bypass a prefix
	// check) with a case-insensitive fallback for Windows volumes.
	absVault, verr := filepath.Abs(vaultDir)
	if verr != nil {
		return "", verr
	}
	// Resolve symlinks on both sides before comparing: on macOS /var is a
	// symlink to /private/var, so a CWD-derived path and a stored path
	// can diverge at the first component while naming the same place.
	// Best-effort: resolved as far as the path exists (the key file may
	// not exist yet — its missing leaf is kept and rejoined).
	av := resolveExistingPath(absVault)
	ap := resolveExistingPath(abs)
	if runtime.GOOS == "windows" {
		av, ap = strings.ToLower(av), strings.ToLower(ap)
	}
	if rel, rerr := filepath.Rel(av, ap); rerr == nil && !strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("key file %s is inside the vault directory — a second factor must live away from the medium it protects", abs)
	}
	// String comparison cannot see filesystem aliasing: on case-insensitive
	// volumes (default APFS, exFAT/FAT media) the same directory answers
	// to many spellings. Compare directory identity along the key file's
	// existing ancestors — if any of them IS the vault directory, the key
	// file is inside it whatever the spelling.
	if vi, verr := os.Stat(av); verr == nil {
		for p := ap; ; {
			if pi, perr := os.Stat(p); perr == nil && os.SameFile(vi, pi) {
				return "", fmt.Errorf("key file %s is inside the vault directory — a second factor must live away from the medium it protects", abs)
			}
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p = parent
		}
	}
	if info, err := os.Stat(abs); err == nil {
		if info.Size() < minKeyfileSize {
			return "", fmt.Errorf("key file %s holds only %d bytes — a second factor must carry real entropy (at least %d bytes)", abs, info.Size(), minKeyfileSize)
		}
		return abs, nil
	}
	key, err := crypto.RandomBytes(32)
	if err != nil {
		return "", err
	}
	// O_EXCL: if the file appears between the stat and the create
	// (restore job, sync tool), use the arrived file instead of
	// clobbering it with fresh random bytes the slot would then be
	// sealed to.
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if info, serr := os.Stat(abs); serr == nil && info.Size() >= minKeyfileSize {
			return abs, nil
		}
		return "", err
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return abs, nil
}

// RemoveSlot drops the slot with the given id from vault.meta. The last
// remaining slot is refused: a vault must keep at least one way in.
func (s *Store) RemoveSlot(id int) error {
	release, err := s.AcquireLock()
	if err != nil {
		return err
	}
	defer release()
	if err := s.refreshMeta(); err != nil {
		return err
	}
	if len(s.meta.Slots) <= 1 {
		return errors.New("refusing to remove the last key slot")
	}
	kept := s.meta.Slots[:0]
	found := false
	for _, sl := range s.meta.Slots {
		if sl.ID == id {
			found = true
			continue
		}
		kept = append(kept, sl)
	}
	if !found {
		return fmt.Errorf("no key slot with id %d", id)
	}
	s.meta.Slots = kept
	return s.rewriteMeta()
}

// Rekey re-seals the DEK under newPass in the slot the current passphrase
// opened, with a fresh salt, nonce, and default parameters. Only
// vault.meta is rewritten: object data is never re-encrypted (the DEK
// itself does not change).
func (s *Store) Rekey(newPass string) error {
	return s.RekeyParams(newPass, crypto.DefaultParams())
}

// RekeyParams is Rekey with caller-chosen Argon2id parameters for the
// re-sealed slot (the per-slot KDF cost selection of the 1.1.0 plan).
func (s *Store) RekeyParams(newPass string, params crypto.Argon2Params) error {
	release, err := s.AcquireLock()
	if err != nil {
		return err
	}
	defer release()
	if err := s.refreshMeta(); err != nil {
		return err
	}
	found := false
	for i, sl := range s.meta.Slots {
		if sl.ID != s.openedSlotID {
			continue
		}
		found = true
		// Preserve the slot's input shape: a second-factor slot re-seals
		// under newPass ‖ keyfile — silently downgrading it to a
		// passphrase-only slot would strip the second factor with no
		// warning.
		secret := newPass
		var keyfileData []byte
		if sl.Input == crypto.InputPassphraseKeyfile {
			if sl.Keyfile == "" {
				return fmt.Errorf("slot %d requires a key file but records none — the slot cannot be re-keyed; contact the vault administrator", sl.ID)
			}
			path := sl.Keyfile
			if !filepath.IsAbs(path) {
				path = filepath.Join(s.dir, path)
			}
			data, err := readLimited(path, 1<<20)
			if err != nil {
				return fmt.Errorf("key file for slot %d: %v", sl.ID, sanitizePath(err.Error()))
			}
			secret = newPass + string(data)
			keyfileData = data
		}
		fresh, err := crypto.SealSlot(sl.ID, secret, s.dek, s.meta.ID, params)
		if err != nil {
			return err
		}
		if sl.Input != "" {
			fresh.Input = sl.Input
			fresh.Keyfile = sl.Keyfile
			if keyfileData != nil {
				// Re-keying reads the current key file: record what it
				// hashes to now, so recognition survives future moves.
				fresh.KeyfileID = crypto.Hash(keyfileData)
			}
		}
		s.meta.Slots[i] = *fresh
		break
	}
	if !found {
		return fmt.Errorf("the key slot this session opened (slot %d) no longer exists — another operation changed the vault's keys; re-open and try again", s.openedSlotID)
	}
	return s.rewriteMeta()
}

// SetBackoff turns the interactive-attempt backoff on or off. Toggling
// must prove knowledge of the current passphrase — the setting changes
// how the CLI treats guesses, so it is not free to flip. Like every meta
// rewrite it takes the writer lock and refreshes under it, so a
// concurrent key operation is never clobbered.
func (s *Store) SetBackoff(on bool) error {
	release, err := s.AcquireLock()
	if err != nil {
		return err
	}
	defer release()
	if err := s.refreshMeta(); err != nil {
		return err
	}
	s.meta.Backoff = on
	return s.rewriteMeta()
}

// BackoffEnabled reports whether the vault at dir opts into the
// interactive-attempt backoff (false for vaults without the field).
func BackoffEnabled(dir string) bool {
	meta, err := ReadMeta(dir)
	if err != nil {
		return false
	}
	return meta.Backoff
}
