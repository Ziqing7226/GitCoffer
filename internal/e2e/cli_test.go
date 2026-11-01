package e2e

// The binary-level CLI contracts: `check`'s exit-code promise (0/1/100,
// with 100 answered before any prompt) and the help feature's exit-code
// contract (usage errors 2, -h exits 0) — both stated in the shipped
// help text and the user guide, so they are pinned against the real
// process exit codes, not just in-process helpers.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	type exitcoder interface{ ExitCode() int }
	if ec, ok := err.(exitcoder); ok {
		return ec.ExitCode()
	}
	t.Fatalf("error without an exit code: %v", err)
	return -1
}

func TestCheckExitCodesEndToEnd(t *testing.T) {
	vaultDir := newVault(t)

	// Healthy vault with the right passphrase: 0.
	_, err := runGitcoffer(t, passphrase+"\n", "check", vaultDir)
	if exitCodeOf(t, err) != 0 {
		t.Fatalf("healthy check = %v, want exit 0", err)
	}

	// Wrong passphrase: 1, with the does-not-open summary.
	out, err := runGitcoffer(t, "wrong\n", "check", vaultDir)
	if code := exitCodeOf(t, err); code != 1 {
		t.Fatalf("wrong passphrase = exit %d, want 1 (out %s)", code, out)
	}
	if !strings.Contains(out, "does not open") {
		t.Fatalf("exit-1 output lacks the summary: %s", out)
	}

	// Missing vault: 100, answered BEFORE any passphrase prompt — an
	// empty stdin must not change the answer.
	out, err = runGitcoffer(t, "", "check", filepath.Join(t.TempDir(), "gone.coffer"))
	if code := exitCodeOf(t, err); code != 100 {
		t.Fatalf("missing vault = exit %d, want 100 (out %s)", code, out)
	}
	if strings.Contains(out, "Passphrase") {
		t.Fatalf("the 100-state prompted for a passphrase: %s", out)
	}

	// A real vault with no passphrase available: 1, and the message must
	// name the failed read, not a generic failure.
	out, err = runGitcoffer(t, "", "check", vaultDir)
	if code := exitCodeOf(t, err); code != 1 {
		t.Fatalf("no passphrase = exit %d, want 1 (out %s)", code, out)
	}
	if !strings.Contains(out, "could not read a passphrase") {
		t.Fatalf("unreadable-passphrase output lacks the explanation: %s", out)
	}

	// A fallback-served vault (damaged newest generation) warns on
	// stderr but still exits 0 — a recoverable vault must not read as
	// missing to a script. The fresh vault has one generation only, so
	// push first: a fallback needs an older generation to serve.
	pushSrc := newRepo(t, "checksrc")
	git(t, pushSrc, nil, "remote", "add", "origin", vaultURL(vaultDir))
	git(t, pushSrc, nil, "push", "-q", "-u", "origin", "main")
	gen := openVault(t, vaultDir).ManifestNum()
	path := filepath.Join(vaultDir, "manifest."+itoa(gen))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x80
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = runGitcoffer(t, passphrase+"\n", "check", vaultDir)
	if code := exitCodeOf(t, err); code != 0 {
		t.Fatalf("fallback-served vault = exit %d, want 0 (out %s)", code, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "unreadable") {
		t.Fatalf("fallback warning missing: %s", out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestHelpExitCodesEndToEnd(t *testing.T) {
	// -h exits 0 with the full help.
	out, err := runGitcoffer(t, "", "-h")
	if exitCodeOf(t, err) != 0 {
		t.Fatalf("-h = %v, want exit 0", err)
	}
	if !strings.Contains(out, "COMMANDS") {
		t.Fatalf("-h output lacks the command list: %s", out[:200])
	}
	// Bare invocation: usage on stderr, exit 2.
	_, err = runGitcoffer(t, "")
	if code := exitCodeOf(t, err); code != 2 {
		t.Fatalf("bare invocation = exit %d, want 2", code)
	}
	// Unknown command: exit 2.
	_, err = runGitcoffer(t, "", "frobnicate")
	if code := exitCodeOf(t, err); code != 2 {
		t.Fatalf("unknown command = exit %d, want 2", code)
	}
	// A bad flag is a usage error too: exit 2, per the help text.
	_, err = runGitcoffer(t, "", "status", "--bogus")
	if code := exitCodeOf(t, err); code != 2 {
		t.Fatalf("bad flag = exit %d, want 2", code)
	}
}
