package vault

// Key-file content recognition (the migratable key-file paths item of
// the 1.1.0 plan): a slot records the SHA-256 of its key file's bytes,
// and when the recorded path no longer exists — the vault-plus-key-file
// set moved between machines or drive letters — the reader recognizes
// the file by content next to the vault instead of locking the slot out.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
)

func TestKeyfileRecognitionAfterMove(t *testing.T) {
	home := t.TempDir() // "the old machine"
	vaultDir := filepath.Join(home, "usb", "project.coffer")
	s, err := CreateFormat(vaultDir, "pass", weakParams(), crypto.CurrentFormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(home, "usb", "coffer.key") // next to the vault
	if err := os.WriteFile(keyfile, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSlot("second", keyfile); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, sl := range s.Meta().Slots {
		if sl.KeyfileID != "" {
			id = sl.KeyfileID
		}
	}
	if id != crypto.Hash([]byte("0123456789abcdef0123456789abcdef")) {
		t.Fatalf("AddSlot did not record the key-file id: %q", id)
	}

	// The whole set moves to "a new machine": old paths gone.
	newHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(newHome, "usb"), 0o755); err != nil {
		t.Fatal(err)
	}
	newVault := filepath.Join(newHome, "usb", "project.coffer")
	if err := os.Rename(vaultDir, newVault); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(keyfile, filepath.Join(newHome, "usb", "coffer.key")); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(newVault, "second")
	if err != nil {
		t.Fatalf("moved vault does not open via key-file recognition: %v", err)
	}
	if got := s2.OpenedSlotID(); got != 1 {
		t.Fatalf("opened slot = %d, want the key-file slot 1", got)
	}

	// Recognition is by content, not presence: a different file next to
	// the vault must not satisfy the slot.
	if err := os.Remove(filepath.Join(newHome, "usb", "coffer.key")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newHome, "usb", "coffer.key"), []byte("different-bytes-entirely-0123"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(newVault, "second"); err == nil || !strings.Contains(err.Error(), "no key slot accepts") {
		t.Fatalf("wrong key-file content accepted: %v", err)
	}

	// The passphrase slot still opens the moved vault throughout.
	if _, err := Open(newVault, "pass"); err != nil {
		t.Fatalf("passphrase slot broken by the move: %v", err)
	}
}

func TestKeyfileContentChangeAtRecordedPathRejected(t *testing.T) {
	home := t.TempDir()
	vaultDir := filepath.Join(home, "v.coffer")
	s, err := CreateFormat(vaultDir, "pass", weakParams(), crypto.CurrentFormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(home, "coffer.key")
	os.WriteFile(keyfile, []byte("0123456789abcdef0123456789abcdef"), 0o600)
	if _, err := s.AddSlot("second", keyfile); err != nil {
		t.Fatal(err)
	}
	// The file at the recorded path is replaced with different bytes.
	os.WriteFile(keyfile, []byte("fedcba9876543210fedcba9876543210"), 0o600)
	_, err = Open(vaultDir, "second")
	if err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("changed key-file content accepted: %v", err)
	}
}

func TestRekeyRefreshesKeyfileID(t *testing.T) {
	home := t.TempDir()
	vaultDir := filepath.Join(home, "v.coffer")
	s, err := CreateFormat(vaultDir, "pass", weakParams(), crypto.CurrentFormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(home, "coffer.key")
	os.WriteFile(keyfile, []byte("0123456789abcdef0123456789abcdef"), 0o600)
	if _, err := s.AddSlot("second", keyfile); err != nil {
		t.Fatal(err)
	}
	// Rotate the passphrase OF THE FIRST SLOT (the one this session
	// opened), then check the key-file slot's id survives the rewrite.
	if err := s.Rekey("third"); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMeta(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sl := range meta.Slots {
		if sl.KeyfileID != "" {
			found = true
			if sl.KeyfileID != crypto.Hash([]byte("0123456789abcdef0123456789abcdef")) {
				t.Fatalf("key-file id drifted after rekey: %s", sl.KeyfileID)
			}
		}
	}
	if !found {
		t.Fatal("key-file id lost after rekey")
	}
	if _, err := Open(vaultDir, "second"); err != nil {
		t.Fatalf("key-file slot broken after rekey: %v", err)
	}
}

// TestPreRecognitionSlotGetsNoRecognition pins the honest boundary: a
// slot without keyfile_id (made by a pre-1.1.0 binary, or stripped) is
// NOT recognized by content — the file sitting next to the vault does
// not satisfy it.
func TestPreRecognitionSlotGetsNoRecognition(t *testing.T) {
	home := t.TempDir()
	vaultDir := filepath.Join(home, "v.coffer")
	s, err := CreateFormat(vaultDir, "pass", weakParams(), crypto.CurrentFormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(home, "coffer.key")
	if err := os.WriteFile(keyfile, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSlot("second", keyfile); err != nil {
		t.Fatal(err)
	}
	// Strip the recorded hash — the pre-1.1.0 slot shape.
	s2, err := Open(vaultDir, "pass")
	if err != nil {
		t.Fatal(err)
	}
	for i := range s2.meta.Slots {
		s2.meta.Slots[i].KeyfileID = ""
	}
	if err := s2.rewriteMeta(); err != nil {
		t.Fatal(err)
	}

	// The recorded path dies, but a byte-identical copy sits right next
	// to the vault under a new name — recognition territory. A hash-less
	// slot must not use it.
	if err := os.Rename(keyfile, filepath.Join(home, "renamed.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(vaultDir, "second"); err == nil {
		t.Fatal("a pre-recognition slot opened via content recognition")
	}
	// Restoring the file at its recorded path works, as documented.
	if err := os.Rename(filepath.Join(home, "renamed.key"), keyfile); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(vaultDir, "second"); err != nil {
		t.Fatalf("slot broken with the key file back at its recorded path: %v", err)
	}
}

// TestKeyfileRecognizedInsideVault covers the search's second scope: the
// key file copied INTO the vault directory (e.g. the whole drive content
// reorganized) is found there when the recorded path is gone.
func TestKeyfileRecognizedInsideVault(t *testing.T) {
	home := t.TempDir()
	vaultDir := filepath.Join(home, "v.coffer")
	s, err := CreateFormat(vaultDir, "pass", weakParams(), crypto.CurrentFormatVersion)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(home, "coffer.key")
	if err := os.WriteFile(keyfile, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSlot("second", keyfile); err != nil {
		t.Fatal(err)
	}
	// The key file moves into the vault directory; the recorded path dies.
	if err := os.Rename(keyfile, filepath.Join(vaultDir, "coffer.key")); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(vaultDir, "second")
	if err != nil {
		t.Fatalf("key file inside the vault not recognized: %v", err)
	}
	if s2.OpenedSlotID() != 1 {
		t.Fatalf("opened slot %d, want the key-file slot", s2.OpenedSlotID())
	}
}
