package main

// The detailed help behind `gitcoffer help`, `gitcoffer help <command>`,
// and `-h`/`--help`. The text is self-contained on purpose: a person —
// or a program driving the CLI — can operate every command from this
// output alone, without opening the documentation. The short usage()
// block stays on the error paths, where brevency matters.

import (
	"errors"
	"fmt"
	"strings"
)

// errUsage marks a usage error (unknown command, wrong argument count).
// main turns it into exit code 2 — the code scripts branch on — instead
// of the exit 1 that runtime failures use.
var errUsage = errors.New("usage error")

// commandDoc is one command's section of the help text.
type commandDoc struct {
	name    string // canonical name ("init", "key")
	summary string // one-line purpose, shown in the command list
	detail  string // indented paragraphs; usage comes from the registry
}

// commandDocs is the help registry, in display order. Adding a command
// means adding an entry here (and the guard tests keep this list, the
// usage block, and the docs in sync).
var commandDocs = []commandDoc{
	{
		name:    "init",
		summary: "create a new encrypted vault",
		detail: `Create a new, empty vault at <vault-directory>; parent
directories are created as needed. Prompts for a new passphrase twice.
Refuses to run when a vault already exists at the path.`,
	},
	{
		name:    "status",
		summary: "inspect a vault",
		detail: `Shows vault facts first — format version, id, creation
time, key-slot count, manifest generations on disk — without a
passphrase. Then prompts for the passphrase and lists the refs (branches
and tags) and object packs the vault holds. Prints a warning when the
newest manifest generation is unreadable or the manifest chain is
broken; both are reasons to run fsck.`,
	},
	{
		name:    "rekey",
		summary: "change the passphrase of the slot it opens",
		detail: `Prompts for the current passphrase, then for the new one
(twice). Only vault.meta is rewritten; object data is not re-encrypted,
and the other key slots keep working.`,
	},
	{
		name:    "key",
		summary: "manage key slots (add, remove, list)",
		detail: `A vault opens when any one of its key slots matches. Every
slot is independent, so a forgotten passphrase can be replaced with a
remembered one via rekey.

  gitcoffer key add [-keyfile <path>] <vault-directory>
      Add a slot: prompts for the current passphrase, then for the new
      passphrase twice. With -keyfile the slot additionally requires
      that key file; the file is created with random bytes when it does
      not exist. A key-file slot records the key file's exact path — a
      copy of the file at another path does not unlock it. A vault
      holds at most 16 slots.

  gitcoffer key remove <vault-directory> <slot-id>
      Remove one slot by its id (see key list). The last remaining slot
      cannot be removed.

  gitcoffer key list <vault-directory>
      List the slots — id, input type, KDF parameters — without a
      passphrase.

Losing every passphrase and key file makes the vault permanently
unreadable; there is no recovery back door. Keep a second slot (or a
backed-up key file) on any vault that matters.`,
	},
	{
		name:    "gc",
		summary: "report reclaimable space, or reclaim it with --prune",
		detail: `Prompts for the passphrase, then reports space the vault
could reclaim: object files referenced by no manifest generation, temp
files left by interrupted writes, and manifest generations beyond the
vault's kept bound. Without --prune nothing is deleted. With --prune the
listed items are removed. gc never deletes when any manifest generation
fails to decrypt — repair the vault with fsck first.`,
	},
	{
		name:    "fsck",
		summary: "verify every structure of the vault",
		detail: `Prompts for the passphrase, then verifies the whole vault:
key-slot shapes, manifest authentication and chain, the ref inventory,
and every object's checksum and plaintext size. Prints each finding as
it goes and exits non-zero when errors are found. Run it after a big
push, or about once a month — a failing drive is caught early, while
recovery is still easy.`,
	},
	{
		name:    "doctor",
		summary: "check the environment and the vault (no passphrase)",
		detail: `Runs without a passphrase. Checks the environment — git
version, whether git-remote-coffer is discoverable on PATH — and the
vault: vault.meta shape, key-file presence, filesystem traits (for
example the FAT-family 4 GiB single-file cap), leftover locks and temp
files. Failures exit non-zero; warnings do not.`,
	},
	{
		name:    "check",
		summary: "probe a vault for scripts (exit 0 / 1 / 100)",
		detail: `Prompts for the passphrase, then answers one question: does
the vault open? The answer is the exit code — 0 the vault opened, 1 the
path holds a vault that does not open (wrong passphrase or damage), 100
not a vault or unreachable (a drive can be unplugged). Serving an older
manifest generation prints a warning but still exits 0: a recoverable
vault must not read as missing. fsck verifies every structure; check
only proves the vault opens. The user guide's Maintenance section has
the cron and Task Scheduler recipes.`,
	},
	{
		name:    "move",
		summary: "relocate a vault directory (source never modified)",
		detail: `Moves the vault to <new-path>. The source is never modified,
so the worst outcome is a cleaned-up partial target. Refused when the
source is not a vault, is being written to, or the target exists
non-empty (or is itself a vault) or lies inside the source.

On one filesystem the move is a single atomic rename. Across
filesystems the vault is copied under its writer lock and the copy is
verified (opened and fscked) before success is reported; only then does
the command ask whether to remove the source — declining, or a
non-interactive run, keeps it as the natural backup.

After the move, every clone needs one line (printed for you):
git remote set-url origin "coffer::<new-path>". Key-file slots whose
recorded path the move invalidates are flagged, and a target on an
online or synced filesystem (network share, cloud folder) gets a word
of caution.`,
	},
	{
		name:    "export-bundle",
		summary: "export the vault as a plain git bundle",
		detail: `Prompts for the passphrase, then decrypts the vault into
<output.bundle> — a standard git bundle any git can clone from, with no
Coffer installed. This is the exit path back to plain git. The output
file is unencrypted; store it accordingly. An existing file is never
overwritten.`,
	},
	{
		name:    "version",
		summary: "print the build version",
		detail: `Prints one line: the release version, or the commit a
source build came from. With --json prints {"version":"..."} instead,
for scripts and editors.`,
	},
	{
		name:    "completion",
		summary: "print a shell completion script",
		detail: `Prints a completion script for bash, zsh, fish, or
powershell. The user guide's Install section shows where each shell
loads it from.`,
	},
	{
		name:    "help",
		summary: "print this help",
		detail: `With no arguments prints the full help — every command,
how passphrase prompts behave, and the exit codes. With a command name
prints that command's section only.`,
	},
}

// findCommandDoc looks up a command by name; "key add" and friends fall
// back to their parent command's section.
func findCommandDoc(name string) *commandDoc {
	for i := range commandDocs {
		if commandDocs[i].name == name {
			return &commandDocs[i]
		}
	}
	return nil
}

// helpUsage is the invocation line shown for one command.
func helpUsage(name string) string {
	switch name {
	case "init", "status", "rekey", "fsck", "doctor", "check":
		return "gitcoffer " + name + " <vault-directory>"
	case "move":
		return "gitcoffer move <vault-directory> <new-path>"
	case "key":
		return "gitcoffer key add [-keyfile <path>] <vault-directory>\n    " +
			"gitcoffer key remove <vault-directory> <slot-id>\n    " +
			"gitcoffer key list <vault-directory>"
	case "gc":
		return "gitcoffer gc [--prune] <vault-directory>"
	case "export-bundle":
		return "gitcoffer export-bundle <vault-directory> <output.bundle>"
	case "version":
		return "gitcoffer version [--json]"
	case "completion":
		return "gitcoffer completion <bash|zsh|fish|powershell>"
	case "help":
		return "gitcoffer help [<command>]"
	}
	return "gitcoffer " + name
}

const helpOverview = `GitCoffer stores a git repository inside an encrypted directory
called a vault. A vault is a plain folder on any disk you choose — a USB
drive, a second internal disk, a mounted share. Git talks to it through
a remote URL of the form

    coffer::<vault-directory>

The coffer:: scheme tells git to run the git-remote-coffer helper, which
ships in the same package as this command and must be on PATH (doctor
checks that). Nothing runs as a server and nothing touches the network:
push and fetch read and write files in the vault directory, encrypted
under the vault's passphrase.

A typical first run:

    gitcoffer init /mnt/usb/myproject.coffer              # Linux
    gitcoffer init /Volumes/Backup/myproject.coffer       # macOS
    gitcoffer init D:\backups\myproject.coffer            # Windows

    cd myproject
    git remote add origin "coffer::/mnt/usb/myproject.coffer"
    git push -u origin main
`

const helpTail = `PASSPHRASE PROMPTS

Every passphrase prompt reads from the terminal with echo off. When
stdin is not a terminal — under a script or another program — the same
prompt reads one line from stdin instead, so vaults can be driven
without a human at the keyboard. A passphrase must not be empty and
must not contain control characters. After the passphrase is proven
correct, the helper hands it to git's credential system, so a
configured credential helper can remember it (the user guide lists the
recipes).

EXIT CODES

0    the command succeeded
1    the command ran and failed (wrong passphrase, damaged vault,
     refused operation)
2    usage error: unknown command, wrong argument count, bad flag

Running gitcoffer with no arguments prints the short usage and exits 2.

WORDS THIS HELP USES

vault     the encrypted directory holding one repository
coffer::  the git URL scheme that addresses a vault
slot      one passphrase (plus optional key file) that opens the vault;
          slots are independent — any one of them opening is enough
manifest the encrypted index of refs and object files; each push writes
          a new numbered generation, and older ones are kept for
          recovery

DOCUMENTATION

User guide, format specification, threat model, support matrix:
https://github.com/Ziqing7226/GitCoffer
`

// fullHelp renders the complete `gitcoffer help` output.
func fullHelp() string {
	var b strings.Builder
	b.WriteString("gitcoffer " + versionString() + " — manage encrypted git vaults\n\n")
	b.WriteString("usage: gitcoffer <command> [arguments]\n\n")
	b.WriteString("OVERVIEW\n\n")
	b.WriteString(indent(helpOverview))
	b.WriteString("\n\nCOMMANDS\n\n")
	for i := range commandDocs {
		doc := &commandDocs[i]
		fmt.Fprintf(&b, "%s\n    %s\n\n", helpUsage(doc.name), doc.summary)
		b.WriteString(indent(doc.detail))
		b.WriteString("\n\n")
	}
	b.WriteString(helpTail)
	return b.String()
}

// commandHelp renders the `gitcoffer help <command>` output.
func commandHelp(doc *commandDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n    %s\n\n", helpUsage(doc.name), doc.summary)
	b.WriteString(indent(doc.detail))
	b.WriteString("\n\nRun `gitcoffer help` for every command.\n")
	return b.String()
}

// indent shifts a paragraph block four spaces for the layout above.
func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "    " + l
		}
	}
	return strings.Join(lines, "\n")
}

// helpCmd implements `gitcoffer help [command]`.
func helpCmd(args []string) error {
	if len(args) == 0 {
		fmt.Print(fullHelp())
		return nil
	}
	name := strings.Join(args, " ")
	doc := findCommandDoc(name)
	if doc == nil && len(args) > 1 {
		doc = findCommandDoc(args[0]) // "key add" -> the key section
	}
	if doc == nil {
		return fmt.Errorf("%w: unknown command %q — run `gitcoffer help` to list every command", errUsage, name)
	}
	fmt.Print(commandHelp(doc))
	return nil
}

// usageHint is the pointer line every per-command usage error carries,
// so a bad invocation still leads to the detailed help.
func usageHint(command string) string {
	return "Run `gitcoffer help " + command + "` for what the command does."
}
