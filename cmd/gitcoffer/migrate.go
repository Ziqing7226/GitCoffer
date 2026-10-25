package main

// gitcoffer migrate converts a v1 vault to format v2 in place. v2 binds
// each manifest's generation number into its authentication, so a
// rolled-back manifest (an older generation's bytes re-filed under a
// newer name) is rejected instead of merely warned about. The command is
// explicit on purpose: nothing ever upgrades a vault automatically, and
// the one-way door — binaries older than 1.1.0 refuse a v2 vault — is
// stated before anything is touched.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
	"golang.org/x/term"
)

func migrateCmd(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "confirm the one-way migration without a prompt (for scripts)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gitcoffer migrate [-yes] <vault-directory>")
		fmt.Fprintln(os.Stderr, usageHint("migrate"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := vaultDirArg(fs)

	meta, err := vault.ReadMeta(dir)
	if err != nil {
		return err
	}
	if meta.FormatVersion >= crypto.FormatVersionV2 {
		fmt.Printf("Vault is already on format v%d — nothing to do.\n", meta.FormatVersion)
		return nil
	}

	// The one-way door, stated before anything is touched.
	fmt.Printf("This vault (id %s) is on format v1. Migrating converts it to v2, in place:\n", meta.ID)
	fmt.Println("  - v2 rejects rolled-back manifests at authentication time (v1 only warns after the fact);")
	fmt.Println("  - object data is not re-encrypted, and clones keep working with the same URL;")
	fmt.Println("  - binaries older than 1.1.0 REFUSE a v2 vault. On a drive you hand to another")
	fmt.Println("    machine, migrate only when that machine runs GitCoffer 1.1.0 or newer.")
	confirmed := *yes
	if !confirmed {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("refusing to migrate unattended — read the notes above, then re-run with -yes")
		}
		fmt.Print("Migrate this vault to format v2? [y/N]: ")
		answer, err := readStdinLine()
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			confirmed = true
		}
	}
	if !confirmed {
		fmt.Println("Not migrated — the vault is unchanged.")
		return nil
	}

	pass, err := promptPassword("Passphrase")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Migrating: fsck precondition, then rewriting vault.meta and every manifest generation (object files untouched)")
	if err := vault.Migrate(dir, pass); err != nil {
		if errors.Is(err, vault.ErrAlreadyV2) {
			fmt.Println("Vault is already on format v2 — nothing to do.")
			return nil
		}
		return err
	}
	fmt.Printf("Vault migrated to format v2: %s\n", dir)
	fmt.Println("Please run gitcoffer fsck to confirm the migrated vault; pushes and pulls work unchanged.")
	return nil
}
