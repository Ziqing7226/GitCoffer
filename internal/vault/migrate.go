package vault

// In-place v1 -> v2 migration, the explicit, per-vault command behind
// `gitcoffer migrate` (docs/development.md, 1.1.0 plan). v2 differs from
// v1 in one thing: the manifest AAD additionally binds the generation
// number, turning rollback (refiling an older manifest's ciphertext
// under a newer generation's name) into an authentication failure.
//
// Migration rewrites only vault.meta and the manifest generations:
// object files are untouched (the DEK does not change), so there is no
// doubled disk usage, no URL change for clones, and no old vault to
// dispose of. Ordering keeps every interruption recoverable by running
// the command again:
//
//  1. fsck must pass — a damaged vault is repaired, not migrated;
//  2. meta flips to format_version 2 first. From this moment a 1.0.0
//     binary refuses the vault outright (the one-way door), while this
//     build still reads any not-yet-resealed v1-sealed generation via
//     its v1 fallback;
//  3. every generation is resealed with its generation number in the
//     AAD, same payload bytes (the prev chain hashes the plaintext, so
//     resealing keeps the chain intact), write-temp-and-rename per file;
//  4. the vault is reopened and fscked again before success is reported.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
)

// ErrAlreadyV2 reports a vault that is already on format v2.
var ErrAlreadyV2 = errors.New("vault is already on format v2")

// Migrate converts the v1 vault at dir to format v2 in place. The caller
// is expected to have warned about the one-way door (binaries older than
// 1.1.0 refuse a v2 vault); the migration itself refuses damaged input.
func Migrate(dir, passphrase string) error {
	s, err := Open(dir, passphrase)
	if err != nil {
		return err
	}
	if s.meta.FormatVersion == crypto.FormatVersionV2 {
		return ErrAlreadyV2
	}
	release, err := s.AcquireLock()
	if err != nil {
		return err
	}
	defer release()

	// Damaged input is repaired, not migrated: a fallback-served vault
	// would bake the damage into the resealed generations.
	findings, err := Fsck(dir, passphrase)
	if err != nil {
		return err
	}
	for _, f := range findings {
		if f.Err {
			return fmt.Errorf("fsck reported errors — repair the vault (gitcoffer fsck lists each finding) before migrating")
		}
	}

	// Flip meta first: the v1-fallback read keeps this build working on
	// not-yet-resealed generations, and older binaries stop here instead
	// of half-trusting the result.
	s.meta.FormatVersion = crypto.FormatVersionV2
	if err := s.rewriteMeta(); err != nil {
		return err
	}

	for _, n := range manifestGenerations(dir) {
		if err := s.resealGeneration(n); err != nil {
			return fmt.Errorf("resealing manifest generation %d failed (%v) — run gitcoffer migrate again to finish", n, err)
		}
	}

	// Verify before reporting success: the migrated vault must open on
	// its newest generation and pass fsck clean.
	s2, err := Open(dir, passphrase)
	if err != nil {
		return fmt.Errorf("the migrated vault does not open: %w", err)
	}
	if f := s2.FallbackFromGeneration(); f > 0 {
		return fmt.Errorf("the migrated vault serves a fallback generation (%d) — run gitcoffer migrate again to finish", f)
	}
	findings, err = Fsck(dir, passphrase)
	if err != nil {
		return err
	}
	for _, f := range findings {
		if f.Err {
			return fmt.Errorf("fsck on the migrated vault reported errors — run gitcoffer fsck and migrate again")
		}
	}
	return nil
}

// resealGeneration rewrites one manifest generation with the v2 AAD,
// unless it is already v2-sealed (an interrupted migration re-runs to
// completion). The payload bytes are preserved exactly, so the
// plaintext-based prev chain stays valid.
func (s *Store) resealGeneration(n int) error {
	if err := s.verifyLockHeld(); err != nil {
		return err
	}
	path := filepath.Join(s.dir, fmt.Sprintf("%s%d", manifestPrefix, n))
	data, err := readLimited(path, maxManifestBytes)
	if err != nil {
		return err
	}
	// Already resealed (or committed fresh by this build): nothing to do.
	if _, err := crypto.OpenManifestV2(s.dek, s.meta.ID, n, data); err == nil {
		return nil
	}
	payload, err := crypto.OpenManifest(s.dek, s.meta.ID, data)
	if err != nil {
		return fmt.Errorf("generation %d does not open under either seal: %w", n, err)
	}
	record, err := crypto.SealManifestV2(s.dek, s.meta.ID, n, payload)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.dir, fmt.Sprintf("%s%d", manifestPrefix, n), func(f *os.File) error {
		_, err := f.Write(record)
		return err
	})
}
