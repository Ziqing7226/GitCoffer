package e2e

// gc --repack end to end: deleted history must physically leave the
// vault. A branch with its own file is pushed and deleted; plain gc
// cannot reclaim it (the objects share a pack file with live history);
// after `gitcoffer gc --repack` the deleted tip is absent from the
// inventory, the old object files are gone, and the surviving refs
// round-trip untouched.

import (
	"fmt"
	"os"
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
