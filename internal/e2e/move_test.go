package e2e

// gitcoffer move end to end: a real vault with real history moves to a
// new path, the clone is re-pointed with the printed set-url line, and
// pushes and pulls against the new location round-trip. Cross-filesystem
// copies need a second mount and stay at the unit level (copyVaultVerified).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoveVaultEndToEnd(t *testing.T) {
	vaultDir := newVault(t)
	src := newRepo(t, "src")
	git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))
	git(t, src, nil, "push", "-q", "-u", "origin", "main")
	tip := rev(t, src, "main")

	moved := filepath.Join(t.TempDir(), "moved.coffer")
	out, err := runGitcoffer(t, "", "move", vaultDir, moved)
	if err != nil {
		t.Fatalf("gitcoffer move: %v\n%s", err, out)
	}
	if !strings.Contains(out, `git remote set-url origin "coffer::`+moved+`"`) {
		t.Fatalf("move report lacks the set-url line:\n%s", out)
	}
	if _, err := os.Stat(vaultDir); !os.IsNotExist(err) {
		t.Fatal("source still present after a same-filesystem move")
	}

	// The clone follows the printed line and keeps working.
	git(t, src, nil, "remote", "set-url", "origin", vaultURL(moved))
	commit(t, src, "file.txt", "one\ntwo\n", "commit two")
	git(t, src, nil, "push", "-q", "origin", "main")
	if got := openVault(t, moved).Manifest().Refs["refs/heads/main"].OID; got != rev(t, src, "main") {
		t.Fatal("push after move did not land at the new path")
	}

	// A fresh clone from the new path restores the full history.
	fresh := filepath.Join(t.TempDir(), "fresh")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(moved), fresh)
	if rev(t, fresh, "main") != rev(t, src, "main") || rev(t, fresh, "main") == tip {
		t.Fatal("clone from the moved vault lost history")
	}
}
