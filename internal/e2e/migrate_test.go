package e2e

// The v1→v2 migration end to end: a real v1 vault with real history
// (created by vault.CreateFormat, the exported way to build a v1 vault
// now that init creates v2), migrated through the CLI, still serving
// pushes and pulls. status prints the upgrade hint before, and the
// format line flips after.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

func TestMigrateV1VaultEndToEnd(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "v1.coffer")
	if _, err := vault.CreateFormat(vaultDir, passphrase, weakParams, crypto.FormatVersion); err != nil {
		t.Fatal(err)
	}

	src := newRepo(t, "src")
	git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))
	git(t, src, nil, "push", "-q", "-u", "origin", "main")

	// status shows the v1 hint before the migration.
	out, err := runGitcoffer(t, passphrase+"\n", "status", vaultDir)
	if err != nil {
		t.Fatalf("status on v1 vault: %v\n%s", err, out)
	}
	if !strings.Contains(out, "format: v1") || !strings.Contains(out, "migrate") {
		t.Fatalf("status lacks the v1 hint:\n%s", out)
	}

	// migrate: non-interactive runs refuse without -yes; with it, the
	// one-line flow needs only the passphrase.
	out, err = runGitcoffer(t, passphrase+"\n", "migrate", vaultDir)
	if err == nil || !strings.Contains(out, "-yes") {
		t.Fatalf("unattended migrate not refused:\n%s (%v)", out, err)
	}
	out, err = runGitcoffer(t, passphrase+"\n", "migrate", "-yes", vaultDir)
	if err != nil {
		t.Fatalf("gitcoffer migrate -yes: %v\n%s", err, out)
	}
	if !strings.Contains(out, "migrated to format v2") {
		t.Fatalf("migrate output lacks confirmation:\n%s", out)
	}

	// The vault still serves the full flow, and pushes stay on v2.
	commit(t, src, "file.txt", "one\ntwo\n", "commit two")
	git(t, src, nil, "push", "-q", "origin", "main")
	if got := openVault(t, vaultDir).Manifest().Refs["refs/heads/main"].OID; got != rev(t, src, "main") {
		t.Fatal("push after migration did not land")
	}
	fresh := filepath.Join(t.TempDir(), "fresh")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(vaultDir), fresh)
	if rev(t, fresh, "main") != rev(t, src, "main") {
		t.Fatal("clone after migration lost history")
	}

	out, err = runGitcoffer(t, passphrase+"\n", "status", vaultDir)
	if err != nil || !strings.Contains(out, "format: v2") {
		t.Fatalf("status after migration shows no v2:\n%s (%v)", out, err)
	}
	if strings.Contains(out, "hint") {
		t.Fatalf("v2 vault still shows the migration hint:\n%s", out)
	}
}
