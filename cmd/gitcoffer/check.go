package main

// gitcoffer check: a three-state probe for scripts and schedulers, in
// the style of git-remote-gcrypt's --check. The exit code is the
// answer:
//
//	0    the vault exists and opens with the supplied passphrase
//	1    the path holds a vault that does not open (wrong passphrase
//	     or damage)
//	100  not a vault — or the path is unreachable (drive unplugged)
//
// check answers one question: does the vault open? Deep verification of
// every structure is fsck's job; the user guide's scheduling recipe
// pairs the two.

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

// The three-state contract scripts branch on.
const (
	checkOpens        = 0
	checkDoesNotOpen  = 1
	checkNotVaultCode = 100
)

// checkState is the pure decision core of `gitcoffer check`, separated
// from the exit machinery so tests can drive it in-process. checkState
// assumes a passphrase is available; checkCmd answers the 100-state
// before prompting (checkNotVault), so an unreachable path reports 100
// even when no passphrase can be supplied.
func checkState(dir, pass string) (code int, summary string, warnings []string) {
	meta, err := vault.ReadMeta(dir)
	if err != nil {
		if errors.Is(err, vault.ErrNotVault) {
			return checkNotVaultCode, fmt.Sprintf("not a vault: %v", err), nil
		}
		// vault.meta is present but fails validation or reading — a vault
		// directory that does not open, including a format this build is
		// too old for.
		return checkDoesNotOpen, fmt.Sprintf("vault does not open: %v", err), nil
	}
	s, err := vault.Open(dir, pass)
	if err != nil {
		return checkDoesNotOpen, fmt.Sprintf("vault does not open: %v", err), nil
	}
	// The vault opens. Serving an older generation is damage worth
	// reporting, but not a different answer: scripts would otherwise
	// treat a recoverable vault as missing.
	if f := s.FallbackFromGeneration(); f > 0 {
		warnings = append(warnings, fmt.Sprintf("newest manifest generation %d is unreadable - showing generation %d; run gitcoffer fsck",
			f, s.ManifestNum()))
	}
	if g := s.ChainBrokenGeneration(); g > 0 {
		warnings = append(warnings, fmt.Sprintf("manifest generation %d does not chain to its predecessor - possible rollback; run gitcoffer fsck", g))
	}
	return checkOpens, fmt.Sprintf("vault opens: format v%d, %d ref(s), manifest generation %d",
		meta.FormatVersion, len(s.Manifest().Refs), s.ManifestNum()), warnings
}

// checkNotVault reports the 100-state without needing the passphrase,
// so an unreachable path answers 100 even when stdin cannot supply a
// prompt line (an unplugged drive, a scheduler with no input).
func checkNotVault(dir string) (bool, string) {
	_, err := vault.ReadMeta(dir)
	if err != nil && errors.Is(err, vault.ErrNotVault) {
		return true, fmt.Sprintf("not a vault: %v", err)
	}
	return false, ""
}

func checkCmd(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gitcoffer check <vault-directory>")
		fmt.Fprintln(os.Stderr, usageHint("check"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := vaultDirArg(fs)
	if notVault, msg := checkNotVault(dir); notVault {
		fmt.Println(msg)
		os.Exit(checkNotVaultCode)
	}
	pass, err := promptPassword("Passphrase")
	if err != nil {
		return err
	}
	code, summary, warnings := checkState(dir, pass)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "gitcoffer: warning: %s\n", w)
	}
	fmt.Println(summary)
	if code != checkOpens {
		os.Exit(code)
	}
	return nil
}
