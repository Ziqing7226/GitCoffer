package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

func TestCheckThreeStates(t *testing.T) {
	// 100: an empty directory is not a vault, and neither is a path
	// that does not exist (a drive can be unplugged).
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "gone")} {
		code, summary, _ := checkState(dir, "pass")
		if code != checkNotVaultCode {
			t.Errorf("checkState(%q) = %d, want %d (%s)", dir, code, checkNotVaultCode, summary)
		}
	}

	// A fresh vault opens with its passphrase.
	dir := t.TempDir()
	if _, err := vault.Create(dir, "hunter2", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	code, summary, warnings := checkState(dir, "hunter2")
	if code != checkOpens {
		t.Fatalf("correct passphrase: code %d (%s)", code, summary)
	}
	if !strings.Contains(summary, "generation 1") || len(warnings) != 0 {
		t.Errorf("happy path summary/warnings: %q %v", summary, warnings)
	}

	// 1: same vault, wrong passphrase.
	code, summary, _ = checkState(dir, "wrong")
	if code != checkDoesNotOpen || !strings.Contains(summary, "does not open") {
		t.Errorf("wrong passphrase: code %d (%s)", code, summary)
	}

	// 100: a garbage vault.meta is not a vault (ReadMeta classifies a
	// non-JSON file as ErrNotVault — same thing a random file is).
	garbage := t.TempDir()
	if err := os.WriteFile(filepath.Join(garbage, "vault.meta"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, _ = checkState(garbage, "pass")
	if code != checkNotVaultCode {
		t.Errorf("garbage meta: code %d, want %d", code, checkNotVaultCode)
	}

	// 1: a well-formed vault.meta that this build cannot use — a newer
	// format version. It is a vault directory; it does not open.
	newer := t.TempDir()
	meta := `{"format_version":99,"id":"` + strings.Repeat("a", 32) + `","created":"2026-01-01T00:00:00Z","slots":[{"id":1,"input":"passphrase","kdf":{"algo":"argon2id","m":65536,"t":3,"p":4},"salt":"` + strings.Repeat("b", 32) + `","wrapped_dek":"x"}]}`
	if err := os.WriteFile(filepath.Join(newer, "vault.meta"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	code, summary, _ = checkState(newer, "pass")
	if code != checkDoesNotOpen || !strings.Contains(summary, "not supported by this build") {
		t.Errorf("newer format: code %d (%s)", code, summary)
	}
}

// The 100-state must be answerable without a passphrase — an
// unreachable path reports itself even when no prompt line exists.
func TestCheckNotVaultWithoutPassphrase(t *testing.T) {
	notVault, msg := checkNotVault(filepath.Join(t.TempDir(), "gone"))
	if !notVault || !strings.Contains(msg, "not a vault") {
		t.Errorf("checkNotVault(missing) = %v %q", notVault, msg)
	}
	dir := t.TempDir()
	if _, err := vault.Create(dir, "hunter2", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	if notVault, _ := checkNotVault(dir); notVault {
		t.Error("checkNotVault reported an existing vault as not-a-vault")
	}
}
