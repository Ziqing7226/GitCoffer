package main

// gitcoffer backoff: the opt-in interactive-attempt backoff from the
// 1.1.0 plan, honestly scoped. Guessing a vault's passphrase is an
// offline attack — the attacker runs their own tools against the sealed
// slot, where nothing we ship can bind them, and any counter stored on
// the medium can be rewound. What CAN be slowed is the casual try-it
// path: gitcoffer's own prompts, for a human at a terminal, re-ask with
// a progressive delay after wrong attempts. Toggling requires opening
// the vault with the current passphrase. No lockout, no self-destruct:
// availability is not traded for security theater.

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func backoffCmd(args []string) error {
	fs := flag.NewFlagSet("backoff", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gitcoffer backoff <on|off> <vault-directory>")
		fmt.Fprintln(os.Stderr, usageHint("backoff"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		os.Exit(2)
	}
	mode := strings.ToLower(fs.Arg(0))
	dir := fs.Arg(1)
	var on bool
	switch mode {
	case "on":
		on = true
	case "off":
		on = false
	default:
		return fmt.Errorf("%w: first argument must be on or off, not %q", errUsage, mode)
	}

	s, _, err := openVaultInteractive(dir)
	if err != nil {
		return err
	}
	if err := s.SetBackoff(on); err != nil {
		return err
	}
	if on {
		fmt.Println("Interactive backoff on: gitcoffer's own passphrase prompts now re-ask with a progressive delay after wrong attempts (terminals only).")
		fmt.Println("This slows a casual try-it, nothing more — an attacker guessing offline runs their own tools and is unaffected.")
	} else {
		fmt.Println("Interactive backoff off: gitcoffer's own prompts ask once, as before.")
	}
	return nil
}
