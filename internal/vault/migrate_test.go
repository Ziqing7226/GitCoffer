package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
)

func manifestName(n int) string { return fmt.Sprintf("%s%d", manifestPrefix, n) }

// resealFor re-seals one generation's payload under another generation
// number: the file a drifting or concurrent writer legitimately leaves
// on a v2 vault, where copying bytes between generations does not
// authenticate (the generation is bound into the AAD).
func resealFor(t *testing.T, s *Store, from, to int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(s.dir, manifestName(from)))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := crypto.OpenManifestV2(s.dek, s.meta.ID, from, raw)
	if err != nil {
		if payload, err = crypto.OpenManifest(s.dek, s.meta.ID, raw); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := crypto.SealManifestV2(s.dek, s.meta.ID, to, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, manifestName(to)), rec, 0o600); err != nil {
		t.Fatal(err)
	}
}

// commitGeneration advances the vault by one manifest generation: one
// stored object, one pack entry, and one ref pointing at it — a state
// fsck accepts, so tests exercise migration over real history.
func commitGeneration(t *testing.T, s *Store, mark byte) {
	t.Helper()
	release, err := s.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	payload := "pack-" + string(mark)
	name, err := s.WriteObject(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	next := *s.Manifest()
	next.Packs[name] = PackInfo{
		SHA256:  crypto.Hash([]byte(payload)),
		Size:    int64(len(payload)),
		Objects: []string{oidOf(mark)},
	}
	next.Refs = map[string]RefVal{"refs/heads/main": {OID: oidOf(mark)}}
	if err := s.Commit(&next); err != nil {
		t.Fatal(err)
	}
}

func oidOf(c byte) string { return strings.Repeat(string(c), 40) }

func TestMigrateV1ToV2(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	s, err := CreateFormat(dir, "pass123", crypto.DefaultParams(), crypto.FormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	commitGeneration(t, s, 'a')
	commitGeneration(t, s, 'b')

	if err := Migrate(dir, "wrong-pass"); err == nil {
		t.Fatal("migrate accepted a wrong passphrase")
	}
	if err := Migrate(dir, "pass123"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Header flipped; content preserved; fsck clean.
	meta, err := ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FormatVersion != crypto.FormatVersionV2 {
		t.Fatalf("format version = %d, want 2", meta.FormatVersion)
	}
	s2, err := Open(dir, "pass123")
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Manifest().Refs["refs/heads/main"].OID; got != oidOf('b') {
		t.Fatalf("refs after migrate = %q, want the committed tip", got)
	}
	findings, err := Fsck(dir, "pass123")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Err {
			t.Fatalf("fsck on migrated vault: %s", f)
		}
	}

	// Every generation on disk is v2-sealed now (no v1 fallback needed).
	for _, n := range GenerationNums(dir) {
		raw, err := os.ReadFile(filepath.Join(dir, manifestName(n)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := crypto.OpenManifestV2(s2.dek, meta.ID, n, raw); err != nil {
			t.Errorf("generation %d is not v2-sealed after migration: %v", n, err)
		}
	}

	// Idempotent: a re-run reports already-v2.
	if err := Migrate(dir, "pass123"); err != ErrAlreadyV2 {
		t.Fatalf("second migrate = %v, want ErrAlreadyV2", err)
	}
}

// TestRollbackRefilingV1SilentV2Rejected is the headline v2 property:
// copying an older generation's ciphertext over a newer generation's
// file. In a v1 vault the refiled record opens — the vault silently
// serves the old state as the new generation. In a v2 vault the
// generation is bound into the AAD, so the refiled record fails
// authentication and the reader falls back with a warning.
func TestRollbackRefilingV1SilentV2Rejected(t *testing.T) {
	build := func(t *testing.T, format int) *Store {
		dir := filepath.Join(t.TempDir(), "v.coffer")
		s, err := CreateFormat(dir, "pass123", crypto.DefaultParams(), format)
		if err != nil {
			t.Fatal(err)
		}
		commitGeneration(t, s, 'a')
		commitGeneration(t, s, 'b')
		if format == crypto.FormatVersionV2 {
			// v2 vaults are already v2 everywhere; nothing more to do.
		}
		// The attack: re-file generation 2's bytes under generation 3.
		// (kept=2, so generations 2 and 3 are the ones on disk.)
		gen2, err := os.ReadFile(filepath.Join(s.dir, manifestName(2)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(s.dir, manifestName(3)), gen2, 0o600); err != nil {
			t.Fatal(err)
		}
		s2, err := Open(dir, "pass123")
		if err != nil {
			t.Fatal(err)
		}
		return s2
	}

	v1 := build(t, crypto.FormatVersion)
	if v1.FallbackFromGeneration() != 0 {
		t.Fatalf("v1 flagged the refiled manifest (%d) — expected the silent accept that motivates v2", v1.FallbackFromGeneration())
	}
	if got := v1.Manifest().Refs["refs/heads/main"].OID; got != oidOf('a') {
		t.Fatalf("v1 rollback did not serve the old state: %q", got)
	}
	if v1.ManifestNum() != 3 {
		t.Fatalf("v1 served generation %d, want 3 (the refiled file)", v1.ManifestNum())
	}

	v2 := build(t, crypto.FormatVersionV2)
	if v2.FallbackFromGeneration() == 0 {
		t.Fatal("v2 accepted the refiled manifest — generation binding broken")
	}
	if v2.ManifestNum() != 2 {
		t.Fatalf("v2 fell back to generation %d, want 2", v2.ManifestNum())
	}
}

func TestNewVaultsAreV2AndCommitSealsV2(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	s, err := Create(dir, "pass123", crypto.DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	if s.meta.FormatVersion != crypto.CurrentFormatVersion {
		t.Fatalf("new vault format = %d, want %d", s.meta.FormatVersion, crypto.CurrentFormatVersion)
	}
	commitGeneration(t, s, 'a')
	raw, err := os.ReadFile(filepath.Join(dir, manifestName(2)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.OpenManifestV2(s.dek, s.meta.ID, 2, raw); err != nil {
		t.Fatalf("generation 2 of a v2 vault is not v2-sealed: %v", err)
	}
}

func TestReadMetaRejectsFutureVersion(t *testing.T) {
	dir := t.TempDir()
	meta := `{"format_version":3,"id":"` + strings.Repeat("a", 32) + `","created":"2026-01-01T00:00:00Z","slots":[]}`
	if err := os.WriteFile(filepath.Join(dir, metaName), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadMeta(dir)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("ReadMeta(v3) = %v, want unsupported-version refusal", err)
	}
}

// TestMigrateRefusesDamagedVault pins the fsck precondition: a vault
// serving a fallback generation must be repaired, not migrated.
func TestMigrateRefusesDamagedVault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	s, err := CreateFormat(dir, "pass123", crypto.DefaultParams(), crypto.FormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	commitGeneration(t, s, 'a')
	commitGeneration(t, s, 'b')
	// Damage the newest generation (flip a byte in the AEAD tag area).
	gen2 := filepath.Join(dir, manifestName(2))
	raw, err := os.ReadFile(gen2)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x80
	if err := os.WriteFile(gen2, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(dir, "pass123"); err == nil || !strings.Contains(err.Error(), "fsck") {
		t.Fatalf("migrate on a damaged vault = %v, want fsck refusal", err)
	}
}
