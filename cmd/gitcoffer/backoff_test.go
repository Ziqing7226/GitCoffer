package main

// Tests for the brute-force options: per-slot KDF cost selection
// (-kdf), the opt-in interactive backoff, and the retry-with-delay open
// path that honors it.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

func TestParseKDFParams(t *testing.T) {
	def := crypto.DefaultParams()
	cases := []struct {
		spec     string
		m, ti, p uint32
		fail     bool
	}{
		{"", def.M, def.T, def.P, false},
		{"m=131072", 131072, def.T, def.P, false},
		{"m=131072,t=4", 131072, 4, def.P, false},
		{"m=131072,t=4,p=8", 131072, 4, 8, false},
		{"t=65", 0, 0, 0, true},         // above the ceiling
		{"m=8,p=1", 8, def.T, 1, false}, // minimum memory
		{"x=1", 0, 0, 0, true},
		{"m=abc", 0, 0, 0, true},
		{"m", 0, 0, 0, true},
	}
	for _, tc := range cases {
		params, err := parseKDFParams(tc.spec)
		if tc.fail {
			if err == nil {
				t.Errorf("parseKDFParams(%q) accepted", tc.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKDFParams(%q): %v", tc.spec, err)
			continue
		}
		if params.M != tc.m || params.T != tc.ti || params.P != tc.p {
			t.Errorf("parseKDFParams(%q) = m=%d t=%d p=%d, want %d/%d/%d",
				tc.spec, params.M, params.T, params.P, tc.m, tc.ti, tc.p)
		}
	}
}

// feedStdin replaces os.Stdin with a string for prompt-driven tests.
func feedStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	// The package-global stdin line buffer must be reset with the pipe:
	// a reader cached from an earlier test would swallow this input.
	stdinLines = nil
	t.Cleanup(func() { os.Stdin = old })
}

func TestOpenWithRetries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	if _, err := vault.Create(dir, "right", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	noDelay := func(int) {}

	// Without the backoff the classic single shot: one wrong line, one
	// authentication failure.
	feedStdin(t, "wrong\n")
	_, _, err := openWithRetries(dir, false, noDelay)
	if !errors.Is(err, crypto.ErrAuth) {
		t.Fatalf("single-shot open = %v, want auth failure", err)
	}

	// With retries (and no real delay), the second line opens the vault.
	feedStdin(t, "wrong\nright\n")
	s, pass, err := openWithRetries(dir, true, noDelay)
	if err != nil || pass != "right" || s == nil {
		t.Fatalf("retrying open = %v (pass %q)", err, pass)
	}

	// All attempts wrong: the last authentication failure surfaces.
	feedStdin(t, "a\nb\nc\n")
	_, _, err = openWithRetries(dir, true, noDelay)
	if !errors.Is(err, crypto.ErrAuth) {
		t.Fatalf("exhausted retries = %v, want auth failure", err)
	}

	// A damaged vault fails once even with retries on (only
	// authentication failures retry): remove the manifest.
	if err := os.Remove(filepath.Join(dir, "manifest.1")); err != nil {
		t.Fatal(err)
	}
	feedStdin(t, "right\nright\nright\n")
	_, _, err = openWithRetries(dir, true, noDelay)
	if errors.Is(err, crypto.ErrAuth) {
		t.Fatal("structural failure was retried as if it were a wrong passphrase")
	}
}

func TestBackoffCommandRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	if _, err := vault.Create(dir, "right", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	if vault.BackoffEnabled(dir) {
		t.Fatal("backoff on by default")
	}

	feedStdin(t, "right\n")
	out := captureStdout(t, func() {
		if err := backoffCmd([]string{"on", dir}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Interactive backoff on") {
		t.Fatalf("backoff on output: %s", out)
	}
	if !vault.BackoffEnabled(dir) {
		t.Fatal("backoff flag not persisted")
	}

	// key list shows the state.
	feedStdin(t, "")
	out = captureStdout(t, func() {
		if err := keyListCmd([]string{dir}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "interactive backoff: on") {
		t.Fatalf("key list lacks backoff state:\n%s", out)
	}

	// Toggling needs the right passphrase.
	feedStdin(t, "wrong\n")
	if err := backoffCmd([]string{"off", dir}); err == nil {
		t.Fatal("backoff off accepted a wrong passphrase")
	}
	feedStdin(t, "right\n")
	out = captureStdout(t, func() {
		if err := backoffCmd([]string{"off", dir}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Interactive backoff off") || vault.BackoffEnabled(dir) {
		t.Fatalf("backoff off failed: %s", out)
	}

	// Bad mode is a usage error.
	if err := backoffCmd([]string{"sideways", dir}); !errors.Is(err, errUsage) {
		t.Fatalf("backoff sideways = %v, want usage error", err)
	}
}

func TestKDFParamsReachTheSlot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "v.coffer")
	if _, err := vault.Create(dir, "open", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	// key add with a raised cost; the recorded slot must carry it.
	feedStdin(t, "open\nsecond\nsecond\n")
	out := captureStdout(t, func() {
		if err := keyAddCmd([]string{"-kdf", "m=131072,t=4", dir}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "below the recommended default") {
		t.Fatalf("raised cost warned as weaker:\n%s", out)
	}
	meta, err := vault.ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sl := range meta.Slots {
		if sl.KDF.M == 131072 && sl.KDF.T == 4 {
			found = true
		}
	}
	if !found {
		t.Fatal("key add did not record the chosen -kdf cost")
	}

	// A weaker-than-default choice warns.
	other := filepath.Join(t.TempDir(), "v2.coffer")
	if _, err := vault.Create(other, "open", crypto.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	feedStdin(t, "open\nsecond\nsecond\n")
	var gotStderr string
	oldStderr := os.Stderr
	rs, ws, _ := os.Pipe()
	os.Stderr = ws
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 8192)
		var sb strings.Builder
		for {
			n, err := rs.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				gotStderr = sb.String()
				close(done)
				return
			}
		}
	}()
	_ = captureStdout(t, func() {
		if err := keyAddCmd([]string{"-kdf", "m=16384", other}); err != nil {
			t.Error(err)
		}
	})
	ws.Close()
	<-done
	os.Stderr = oldStderr
	if !strings.Contains(gotStderr, "below the recommended default") {
		t.Fatalf("weaker choice did not warn:\n%s", gotStderr)
	}
}
