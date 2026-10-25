package main

// Guards for the detailed help: it is the contract that a reader — or
// a program driving the CLI — can operate every command from
// `gitcoffer help` alone, without opening the documentation. These
// tests pin that the help covers the command surface and states the
// load-bearing mechanics, so the text cannot silently rot.

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var sb strings.Builder
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func TestHelpCoversEveryCommand(t *testing.T) {
	out := captureStdout(t, func() {
		if err := helpCmd(nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, cmd := range implementedCommands {
		if !strings.Contains(out, "gitcoffer "+cmd) {
			t.Errorf("help text does not list `gitcoffer %s`", cmd)
		}
	}
	for i := range commandDocs {
		if !strings.Contains(out, commandDocs[i].name) {
			t.Errorf("help text lost the %q section", commandDocs[i].name)
		}
	}
}

// TestHelpStatesTheMechanics pins the facts an operator needs from the
// help alone: the URL form, the helper, the prompt behavior under a
// non-terminal stdin, the exit codes, and the slot cap.
func TestHelpStatesTheMechanics(t *testing.T) {
	out := captureStdout(t, func() {
		if err := helpCmd(nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, pin := range []string{
		"coffer::",
		"git-remote-coffer",
		"stdin is not a terminal",
		"EXIT CODES",
		"exits 2",
		"at most 16 slots",
	} {
		if !strings.Contains(out, pin) {
			t.Errorf("help text lost the fact %q", pin)
		}
	}
}

func TestHelpOneCommand(t *testing.T) {
	out := captureStdout(t, func() {
		if err := helpCmd([]string{"gc"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"gitcoffer gc [--prune]", "--prune", "every command"} {
		if !strings.Contains(out, want) {
			t.Errorf("help gc lacks %q:\n%s", want, out)
		}
	}
}

func TestHelpSubcommandFallsBackToParent(t *testing.T) {
	out := captureStdout(t, func() {
		if err := helpCmd([]string{"key", "add"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "key add") {
		t.Errorf("help key add does not show the key section:\n%s", out)
	}
}

func TestHelpUnknownCommandIsUsageError(t *testing.T) {
	if err := helpCmd([]string{"bogus"}); !errors.Is(err, errUsage) {
		t.Fatalf("help bogus = %v, want errUsage", err)
	}
}
