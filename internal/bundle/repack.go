package bundle

// Repack rebuilds the vault's object files so they hold only the objects
// reachable from the current manifest's refs — the physical reclamation
// of deleted history (the 1.1.0 plan's gc-repacking item). Today's gc
// works at file granularity: an object file written by a push that also
// carried a since-deleted branch stays referenced as long as any object
// in it is reachable, so the deleted branch's bytes never leave the
// medium. Repack answers that: every pack is decrypted into a scratch
// repository, git rebuilds one pack from the refs' closure, the new pack
// is stored and committed as a new manifest generation, and the old
// generations and packs are removed under the same writer lock.
//
// The trade is space over recovery: after a repack no older manifest
// generation exists to fall back to, and the bytes of deleted history
// are gone. Crash safety holds throughout — objects before manifest
// (spec §6), removals only after the commit — so an interrupted repack
// leaves a vault that opens, with leftovers the next gc (or repack)
// sweeps.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Ziqing7226/GitCoffer/internal/packproc"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

// RepackResult reports what one repack did.
type RepackResult struct {
	Refs             int
	Objects          int      // objects in the rebuilt pack
	NewPack          string   // the rebuilt pack's vault file name
	OldPacks         []string // removed vault files
	OldGenerations   []int    // removed manifest generations
	PlainBytesBefore int64    // plaintext the packs held before
	PlainBytesAfter  int64    // plaintext the rebuilt pack holds
}

// Repack rewrites the vault's pack layer in place. vaultDir must open
// with passphrase, and the newest manifest generation must be readable —
// a fallback-served vault is repaired (fsck), not repacked.
func Repack(vaultDir, passphrase string) (*RepackResult, error) {
	s, err := vault.Open(vaultDir, passphrase)
	if err != nil {
		return nil, err
	}
	if f := s.FallbackFromGeneration(); f > 0 {
		return nil, fmt.Errorf(
			"the newest manifest generation %d is unreadable and an older state is being served — run gitcoffer fsck before repacking", f)
	}
	m := s.Manifest()
	if len(m.Refs) == 0 {
		return nil, fmt.Errorf("the vault holds no refs — nothing to repack")
	}

	repo, cleanup, err := packproc.TempBareRepo()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Revive every stored pack, then recreate the refs.
	if err := revivePacks(s, m, repo); err != nil {
		return nil, err
	}
	refNames := make([]string, 0, len(m.Refs))
	for name := range m.Refs {
		refNames = append(refNames, name)
	}
	sort.Strings(refNames)
	for _, name := range refNames {
		if err := packproc.RunIn(repo, "update-ref", name, m.Refs[name].OID); err != nil {
			return nil, fmt.Errorf("restoring %s: %v", name, err)
		}
	}

	// One pack holding the refs' full closure. Objects of deleted refs
	// are unreachable here — this is where the reclamation happens.
	packPath, err := packFromRefs(repo, m)
	if err != nil {
		return nil, err
	}
	oids, idxCleanup, err := packproc.IndexPack(packPath)
	if err != nil {
		return nil, err
	}
	defer idxCleanup()
	st, err := os.Stat(packPath)
	if err != nil {
		return nil, err
	}

	// Store the rebuilt pack under the writer lock, adopting concurrent
	// state first; a concurrent push between the scan and the lock
	// changes the refs, so abort and let the caller re-run.
	release, err := s.AcquireLock()
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.Reload(); err != nil {
		return nil, err
	}
	cur := s.Manifest()
	if len(cur.Refs) != len(m.Refs) {
		return nil, fmt.Errorf("the vault changed during the repack scan (concurrent push) — nothing was changed; re-run the repack")
	}
	for name, rv := range m.Refs {
		if now, ok := cur.Refs[name]; !ok || now.OID != rv.OID {
			return nil, fmt.Errorf("the vault changed during the repack scan (concurrent push) — nothing was changed; re-run the repack")
		}
	}

	f, err := os.Open(packPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hasher := sha256.New()
	newName, err := s.WriteObject(io.TeeReader(f, hasher))
	if err != nil {
		return nil, err
	}

	// Commit the new generation: same refs, exactly one pack. Format v2
	// vaults seal v2 automatically (Commit honors vault.meta).
	next := *cur
	next.Packs = map[string]vault.PackInfo{
		newName: {
			SHA256:  hex.EncodeToString(hasher.Sum(nil)),
			Size:    st.Size(),
			Objects: oids,
		},
	}
	if err := s.Commit(&next); err != nil {
		return nil, err
	}

	res := &RepackResult{
		Refs:    len(refNames),
		Objects: len(oids),
		NewPack: newName,
	}
	for name, p := range m.Packs {
		res.PlainBytesBefore += p.Size
		if name != newName {
			res.OldPacks = append(res.OldPacks, name)
		}
	}
	res.PlainBytesAfter = st.Size()
	sort.Strings(res.OldPacks)

	// Destructive tail, still under the lock: drop every older
	// generation, then the packs only they referenced. Each step is a
	// plain removal — an interruption leaves harmless leftovers for the
	// next gc or repack to sweep.
	if err := s.VerifyLockHeld(); err != nil {
		return nil, err
	}
	newNum := s.ManifestNum()
	for _, n := range vault.GenerationNums(vaultDir) {
		if n == newNum {
			continue
		}
		if os.Remove(filepath.Join(vaultDir, fmt.Sprintf("manifest.%d", n))) == nil {
			res.OldGenerations = append(res.OldGenerations, n)
		}
	}
	for _, name := range res.OldPacks {
		os.Remove(filepath.Join(vaultDir, "obj", name))
	}
	return res, nil
}

// revivePacks decrypts every pack of manifest m into the scratch
// repository at repo and indexes it — the same revival the fetch path
// performs.
func revivePacks(s *vault.Store, m *vault.Manifest, repo string) error {
	packDir := filepath.Join(repo, "objects", "pack")
	names := make([]string, 0, len(m.Packs))
	for name := range m.Packs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rd, err := s.ReadObject(name)
		if err != nil {
			return err
		}
		dst := filepath.Join(packDir, "pack-"+name+".pack")
		err = writeAllAndClose(rd, dst)
		if err != nil {
			return fmt.Errorf("vault file %s: %v — the medium may be damaged; restore this file from a backup of the vault, then retry", name, err)
		}
		if err := packproc.IndexPackInDir(repo, dst); err != nil {
			return err
		}
	}
	return nil
}

// packFromRefs builds one pack holding everything reachable from the
// manifest's refs, inside the scratch repository.
func packFromRefs(repo string, m *vault.Manifest) (string, error) {
	revs := make([]string, 0, len(m.Refs))
	for _, rv := range m.Refs {
		revs = append(revs, rv.OID)
	}
	tmp, err := os.MkdirTemp("", "coffer-repack-")
	if err != nil {
		return "", err
	}
	packPath := filepath.Join(tmp, "repack.pack")
	cmd := exec.Command("git", "pack-objects", "--stdout", "--revs")
	cmd.Dir = repo
	cmd.Env = packproc.CleanGitEnv()
	cmd.Stdin = newLineReader(revs)
	out, err := os.Create(packPath)
	if err != nil {
		return "", err
	}
	cmd.Stdout = out
	runErr := cmd.Run()
	cerr := out.Close()
	if runErr != nil {
		return "", fmt.Errorf("pack-objects: %v", runErr)
	}
	if cerr != nil {
		return "", cerr
	}
	info, err := os.Stat(packPath)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("pack-objects produced no pack — the refs point at objects missing from the stored packs; run gitcoffer fsck")
	}
	return packPath, nil
}

func newLineReader(lines []string) *strings.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}
