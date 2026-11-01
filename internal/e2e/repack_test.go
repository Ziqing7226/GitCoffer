package e2e

// gc --repack end to end: deleted history must physically leave the
// vault. A branch with its own file is pushed and deleted; plain gc
// cannot reclaim it (the objects share a pack file with live history);
// after `gitcoffer gc --repack` the deleted tip is absent from the
// inventory, the old object files are gone, and the surviving refs
// round-trip untouched.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGcRepackReclaimsDeletedHistory(t *testing.T) {
	vaultDir := newVault(t)
	src := newRepo(t, "src")
	git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))
	git(t, src, nil, "push", "-q", "-u", "origin", "main")

	// A branch with its own file, pushed, then deleted from the vault.
	commit(t, src, "secret.txt", "deleted-branch-payload\n", "commit secret")
	secretTip := rev(t, src, "HEAD")
	git(t, src, nil, "push", "-q", "origin", "HEAD:refs/heads/feature")
	git(t, src, nil, "push", "-q", "origin", ":refs/heads/feature")
	packsBefore := len(openVault(t, vaultDir).Manifest().Packs)

	// Plain gc cannot reclaim the deleted branch: its objects sit inside
	// a pack that live history still references.
	out, err := runGitcoffer(t, passphrase+"\n", "gc", "--prune", vaultDir)
	if err != nil {
		t.Fatalf("gc --prune: %v\n%s", err, out)
	}
	s := openVault(t, vaultDir)
	deletedStillThere := false
	for _, p := range s.Manifest().Packs {
		for _, oid := range p.Objects {
			if oid == secretTip {
				deletedStillThere = true
			}
		}
	}
	if !deletedStillThere {
		t.Fatal("precondition broken: the deleted branch's tip was already gone before the repack")
	}

	// The repack reclaims it.
	out, err = runGitcoffer(t, passphrase+"\n", "gc", "--repack", vaultDir)
	if err != nil {
		t.Fatalf("gc --repack: %v\n%s", err, out)
	}
	if !strings.Contains(out, "repacked 1 ref(s)") {
		t.Fatalf("repack report lacks the summary:\n%s", out)
	}

	s = openVault(t, vaultDir)
	if len(s.Manifest().Packs) != 1 {
		t.Fatalf("packs after repack = %d, want 1", len(s.Manifest().Packs))
	}
	for _, p := range s.Manifest().Packs {
		for _, oid := range p.Objects {
			if oid == secretTip {
				t.Fatal("the deleted branch's tip survived the repack")
			}
		}
	}
	// The old object files are physically gone.
	entries, err := os.ReadDir(filepath.Join(vaultDir, "obj"))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range entries {
		if !e.IsDir() && !strings.HasSuffix(e.Name(), ".tmp") {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("object files on disk after repack = %d (was %d packs before), want 1", files, packsBefore)
	}
	// Recovery generations were dropped with the old packs.
	if gens := vaultGenerationNums(t, vaultDir); len(gens) != 1 {
		t.Fatalf("manifest generations after repack = %v, want exactly the new one", gens)
	}

	// The surviving history round-trips and fsck stays clean.
	fresh := filepath.Join(t.TempDir(), "fresh")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(vaultDir), fresh)
	if rev(t, fresh, "main") != rev(t, src, "main~1") {
		t.Fatal("clone after repack lost the surviving history")
	}
	if _, err := runGit(fresh, nil, "cat-file", "-e", secretTip); err == nil {
		t.Fatal("the deleted branch's object is still cloneable after the repack")
	}
	out, err = runGitcoffer(t, passphrase+"\n", "fsck", vaultDir)
	if err != nil {
		t.Fatalf("fsck after repack: %v\n%s", err, out)
	}

	// Pushes still work on the repacked vault.
	commit(t, src, "file.txt", "one\ntwo\n", "commit two")
	git(t, src, nil, "push", "-q", "origin", "main")
	if got := openVault(t, vaultDir).Manifest().Refs["refs/heads/main"].OID; got != rev(t, src, "main") {
		t.Fatal("push after repack did not land")
	}
}

func vaultGenerationNums(t *testing.T, dir string) []int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var gens []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "manifest.") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(name, "manifest."), "%d", &n); err == nil {
			gens = append(gens, n)
		}
	}
	return gens
}

// runGitcofferEnv is runGitcoffer with extra environment (the crash
// harness rides it).
func runGitcofferEnv(t *testing.T, stdin string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := execCommand(filepath.Join(binDir, "gitcoffer"+exeExt()), args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func execCommand(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }

// Crash safety of the repack, with the same fault-injection discipline
// as the push path: a crash at any write stage leaves a vault that
// opens, holds the pre-repack refs, passes fsck, and converges when the
// repack (or a plain gc) runs again.
func TestGcRepackCrashPoints(t *testing.T) {
	for _, point := range []string{
		"after-object-write",
		"after-object-rename",
		"before-manifest-commit",
		"after-manifest-write",
		"after-manifest-commit",
	} {
		t.Run(point, func(t *testing.T) {
			vaultDir := newVault(t)
			src := newRepo(t, "src")
			git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))
			commit(t, src, "secret.txt", "deleted-branch-payload\n", "commit secret")
			git(t, src, nil, "push", "-q", "-u", "origin", "HEAD:refs/heads/feature")
			git(t, src, nil, "push", "-q", "origin", "HEAD:refs/heads/main")
			git(t, src, nil, "push", "-q", "origin", ":refs/heads/feature")
			mainTip := rev(t, src, "main")

			_, err := runGitcofferEnv(t, passphrase+"\n", []string{"COFFER_CRASH=" + point}, "gc", "--repack", vaultDir)
			if err == nil {
				t.Fatal("crash point did not fire")
			}

			// Invariant: the vault opens, keeps the pre-repack refs, and
			// fsck reports no errors.
			s := openVault(t, vaultDir)
			if got := s.Manifest().Refs["refs/heads/main"].OID; got != mainTip {
				t.Fatalf("refs damaged by the crash: %s", got)
			}
			out, err := runGitcoffer(t, passphrase+"\n", "fsck", vaultDir)
			if err != nil {
				t.Fatalf("fsck after crash: %v\n%s", err, out)
			}

			// Convergence: another repack finishes the job cleanly.
			if _, err := runGitcoffer(t, passphrase+"\n", "gc", "--repack", vaultDir); err != nil {
				t.Fatalf("converging repack: %v", err)
			}
			s = openVault(t, vaultDir)
			if len(s.Manifest().Packs) != 1 {
				t.Fatalf("packs after converging repack = %d, want 1", len(s.Manifest().Packs))
			}
		})
	}
}

// Idempotence and shape: repacking twice is safe, keeps every branch
// and tag, and the plaintext pack never outlives the command (the
// %TEMP% leak the temp cleanup fixed).
func TestGcRepackIdempotentAndClean(t *testing.T) {
	countRepackTemps := func() int {
		matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "coffer-repack-*"))
		return len(matches)
	}

	vaultDir := newVault(t)
	src := newRepo(t, "src")
	commit(t, src, "file.txt", "one\ntwo\n", "commit two")
	git(t, src, nil, "branch", "feature")
	commit(t, src, "feature.txt", "feature\n", "commit feature")
	git(t, src, nil, "tag", "-a", "v1", "-m", "tag")
	git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))
	git(t, src, nil, "push", "-q", "--all", "origin")
	git(t, src, nil, "push", "-q", "--tags", "origin")

	before := countRepackTemps()
	for i := 0; i < 2; i++ {
		if out, err := runGitcoffer(t, passphrase+"\n", "gc", "--repack", vaultDir); err != nil {
			t.Fatalf("repack run %d: %v\n%s", i+1, err, out)
		}
	}
	s := openVault(t, vaultDir)
	if len(s.Manifest().Packs) != 1 {
		t.Fatalf("packs after two repacks = %d, want 1", len(s.Manifest().Packs))
	}
	for _, want := range []string{"refs/heads/main", "refs/heads/feature", "refs/tags/v1"} {
		if _, ok := s.Manifest().Refs[want]; !ok {
			t.Errorf("ref %s lost by the repacks", want)
		}
	}
	// Every ref must survive a fresh clone, tags included.
	fresh := filepath.Join(t.TempDir(), "fresh")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(vaultDir), fresh)
	if rev(t, fresh, "refs/tags/v1") != rev(t, src, "v1") {
		t.Fatal("tag mismatch after repack")
	}
	if after := countRepackTemps(); after != before {
		t.Errorf("repack leaked %d temp dir(s): %d -> %d", after-before, before, after)
	}

	// A fallback-served vault is refused: repair first, repack after.
	// (The repack leaves one generation, so first push another one to
	// damage — a single-generation vault has no fallback state at all.)
	commit(t, src, "file.txt", "one\ntwo\nthree\n", "commit three")
	git(t, src, nil, "push", "-q", "origin", "main")
	gen := openVault(t, vaultDir).ManifestNum()
	path := filepath.Join(vaultDir, fmt.Sprintf("manifest.%d", gen))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x80
	os.WriteFile(path, raw, 0o600)
	out, err := runGitcoffer(t, passphrase+"\n", "gc", "--repack", vaultDir)
	if err == nil || !strings.Contains(out, "fsck") {
		t.Fatalf("repack on a fallback-served vault = %v\n%s, want an fsck refusal", err, out)
	}
}
