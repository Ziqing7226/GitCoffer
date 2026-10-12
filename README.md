<div align="center">
  <img src="docs/assets/coffer.svg" width="140" alt="Coffer logo" />
</div>

<h1 align="center">GitCoffer</h1>

<p align="center">
  <strong>An encrypted git remote on your own disk.</strong><br>
  Push from any git client — including VSCode — straight into a
  password-protected vault on a second disk or USB drive.
</p>

<p align="center">
  <img alt="version 1.0.0" src="https://img.shields.io/badge/version-1.0.0-blue">
  &nbsp;
  <img alt="platforms" src="https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-6e7681">
  &nbsp;
  <img alt="license" src="https://img.shields.io/badge/license-MIT-6e7681">
</p>

<p align="center">
  <img src="docs/assets/workflow.png" alt="git push, encrypted by Argon2id and XChaCha20-Poly1305, stored in a vault on a USB drive" width="720">
</p>

---

Coffer turns any directory on a removable or secondary disk into a fully
encrypted git remote. `git push` works exactly the way it always has — but
the bytes that reach the disk are ciphertext. Whoever finds the drive finds
random-looking files; with the passphrase, the complete repository — every
branch, tag, and commit — is reconstructed from the vault alone.

Vault format v1 is frozen and pinned by published
[test vectors](docs/test-vectors.json). This is the **1.0.0** stable
release — the format the release candidates have been writing all
along, after seven release candidates of fix-and-reverify across three
operating systems and real external media
([how Coffer is tested](#how-coffer-is-tested)).

## Why

<p align="center">
  <img src="docs/assets/why-coffer.png" alt="A vault at the center, surrounded by: no third party, single static binary, cross-platform, native git clients" width="560">
</p>

- **No third party.** The remote is a directory you control — a USB stick,
  an external drive, a second internal disk. Nothing ever leaves your
  hardware; the code contains no networking at all.
- **Encrypted at rest.** The entire repository — objects, refs, history — is
  sealed under passphrase encryption (Argon2id key derivation,
  XChaCha20-Poly1305 AEAD). Possessing the drive is not enough.
- **Stock git, unchanged.** Coffer is a standard
  [git remote helper](docs/architecture.md). No forked git, no wrappers to
  remember, no server to run. If `git push` works, Coffer works — and so does
  the VSCode Sync button.
- **Single static binary.** No GPG, no FUSE, no drivers, no administrator
  rights. The vault is plain files, so exFAT and FAT32 media are fully
  supported.

## How it works

Git delegates transport for unknown URL schemes to a helper binary: a
`coffer::<path>` remote makes git invoke `git-remote-coffer`. On push, the
helper reads the objects straight from your repository, encrypts them, and
writes them into the vault; on fetch it does the reverse, importing
decrypted objects back into your object database. Git never notices the
difference — which is why every git client stays compatible. Requirements:
stock git ≥ 2.30 (where the helper `object-format` capability first
appeared) and the two GitCoffer binaries on `PATH`. The design and its
rationale are in [docs/architecture.md](docs/architecture.md); the on-disk
layout is normative in [docs/format-spec.md](docs/format-spec.md).

## Install

Download a release for your platform — Linux, Windows, macOS, amd64 or
arm64 — from the [releases page](https://github.com/Ziqing7226/GitCoffer/releases),
unpack, and put both `gitcoffer` and `git-remote-coffer` on `PATH` (git finds
the helper through `PATH`; no administrator rights needed). Verify:

```console
$ gitcoffer version
gitcoffer v1.0.0
```

The version line is self-describing: release archives print the
release version, `go install …@vX` prints that exact version, and a
build from the repository reports the commit it was built from.

Or install with the script (downloads the latest release, verifies the
checksum, installs into `~/.local/bin` — see the script header for
options):

```console
$ curl -fsSL https://raw.githubusercontent.com/Ziqing7226/GitCoffer/main/scripts/install.sh | sh
```

Windows PowerShell has the equivalent `scripts/install.ps1`. On macOS,
see the user guide's macOS notes for GUI-app PATH visibility and
Gatekeeper. For shell
completion, run `gitcoffer completion bash` (also `zsh`, `fish`,
`powershell`) and load its output from your shell profile.

Or build from source with Go ≥ 1.27:

```console
$ go install github.com/Ziqing7226/GitCoffer/cmd/gitcoffer@v1.0.0
$ go install github.com/Ziqing7226/GitCoffer/cmd/git-remote-coffer@v1.0.0
```

**Package managers.** A Homebrew tap (macOS / Linux):

```console
$ brew tap Ziqing7226/gitcoffer
$ brew install Ziqing7226/gitcoffer/gitcoffer
```

scoop (Windows):

```powershell
scoop bucket add ziqing7226 https://github.com/Ziqing7226/scoop-bucket
scoop install gitcoffer
```

A winget entry (`winget install Ziqing7226.GitCoffer`) is in review and
lands when the community repository accepts it.

## Quick start

```console
$ gitcoffer init /mnt/usb/myproject.coffer
Enter passphrase for the new vault: ********
Repeat passphrase: ********
Vault created: /mnt/usb/myproject.coffer

$ git remote add origin coffer::/mnt/usb/myproject.coffer
$ git push -u origin main
Enumerating objects: 42, done.
Writing objects: 100% (42/42), done.
To coffer::/mnt/usb/myproject.coffer
 * [new branch]      main -> main
```

From then on, `git pull`, `git fetch`, `git clone`, and VSCode's Sync
button all work against the vault. The passphrase is requested through
git's own credential flow — terminal prompt, native VSCode input box, or a
configured credential helper (`--atomic` and `--force-with-lease` work as
with any remote). On Windows:
`git remote add origin coffer::D:\backups\myproject.coffer`; on macOS:
`coffer::/Volumes/Backup/myproject.coffer`. To clone on another machine,
only git, Coffer, and the passphrase are needed — and "another machine"
includes another operating system: hand the drive to a colleague and
they clone from it on Windows, Linux, or macOS with the passphrase.
`git clone coffer::/mnt/usb/myproject.coffer`.

### Nicer remote URLs

```console
$ git config --global url."coffer::/mnt/usb/".insteadOf "usb://"
$ git remote add origin usb://myproject.coffer
```

## Moving an existing project onto a vault

No import, no conversion — a vault is just another remote.

Already have the project cloned (your current branch and tags):

```console
$ gitcoffer init /mnt/usb/myproject.coffer
$ git remote add vault coffer::/mnt/usb/myproject.coffer
$ git push vault --all     # every branch that exists in this clone
$ git push vault --tags    # every tag
$ git remote set-url origin coffer::/mnt/usb/myproject.coffer   # make it the default
```

Everything on the old host, exactly as it is (all branches, tags, notes):

```console
$ git clone --mirror git@example.com:you/myproject.git
$ git push --mirror coffer::/mnt/usb/myproject.coffer
```

From then on `git push`, `git pull`, and VSCode's Sync button talk to
the vault; on another machine, `git clone
coffer::/mnt/usb/myproject.coffer` is the whole story (Windows:
`coffer::D:\backups\myproject.coffer`; macOS:
`coffer::/Volumes/Backup/myproject.coffer`). Keep the old host as a
second remote, or delete the repository there once you are comfortable —
the vault and your passphrase reconstruct everything.

### The coffer CLI

| Command | Purpose |
|---|---|
| `gitcoffer init <dir>` | create a new vault |
| `gitcoffer status <dir>` | inspect a vault (prompts for the passphrase to show refs and packs) |
| `gitcoffer rekey <dir>` | change the passphrase (data is never re-encrypted) |
| `gitcoffer key add / remove / list` | manage key slots; optional key-file second factor |
| `gitcoffer gc [--prune] <dir>` | report reclaimable space, or reclaim it with `--prune` |
| `gitcoffer fsck <dir>` | verify every structure of the vault |
| `gitcoffer doctor <dir>` | check the environment and the vault |
| `gitcoffer export-bundle <dir> <file>` | export the vault as a plain git bundle — the exit path |
| `gitcoffer version [--json]` | print the build version (optionally as JSON) |
| `gitcoffer completion <bash|zsh|fish|powershell>` | print a shell completion script |

Full walkthrough — keys and recovery, maintenance, troubleshooting:
[docs/user-guide.md](docs/user-guide.md).

## Inside VSCode

Nothing new to learn: the vault is a remote, so the editor's built-in
git tooling behaves exactly as it does with any host. Publishing asks
for the passphrase once, through git's own native prompt — after that,
Sync, the incoming/outgoing counters, and the commit graph all read
from the encrypted vault on your disk. These screenshots show a real
vault on a USB drive, pushed from the editor itself:

<table>
  <tr>
    <td width="50%">
      <img src="docs/assets/vscode-publish.png" width="380" alt="VSCode Source Control view with the Publish Branch button"><br>
      <em>Publish Branch — one button, as with any remote</em>
    </td>
    <td width="50%">
      <img src="docs/assets/vscode-password.png" width="380" alt="VSCode native password prompt for the coffer vault"><br>
      <em>The passphrase arrives through git's native credential prompt</em>
    </td>
  </tr>
  <tr>
    <td>
      <img src="docs/assets/vscode-synced.png" width="380" alt="VSCode Source Control view in sync, 0 down 0 up, origin/main current"><br>
      <em>In sync: 0↓ 0↑, origin/main current</em>
    </td>
    <td>
      <img src="docs/assets/vscode-graph.png" width="380" alt="VSCode built-in commit graph with main and origin/main refs"><br>
      <em>The built-in commit graph, remote refs included</em>
    </td>
  </tr>
</table>

## Security scope

<p align="center">
  <img src="docs/assets/locked-vs-open.png" alt="Left: someone finding the drive sees only scrambled scribbles. Right: with the passphrase, the vault opens into an orderly commit history." width="640">
</p>

Coffer protects **repository data at rest on the remote medium**.

| In scope | Out of scope |
|---|---|
| Lost, stolen, copied, or forensically imaged drives | A host compromised *while* the vault is in use |
| Failure of the original working disk — the vault is a complete backup | A forgotten passphrase — there is no recovery, by design |
| Bit-rot and tampering with vault files (AEAD-verified) | Hiding vault metadata such as file count and approximate sizes |

The full analysis is in [docs/threat-model.md](docs/threat-model.md).

## Choosing an approach

Which copy actually needs encrypting? Servers — including self-hosted
forges like Gitea and GitLab, which keep repositories in plaintext by
design and cover the machine in the rack with full-disk encryption — are
not the weak link. The weak link is the copy that leaves: the portable
drive carried offsite, in bags, between machines, that eventually gets
lost. Once it leaves the server, it is outside every one of those
protections.

Encrypting that copy has four mature shapes, each fitting different
needs — the right choice is whatever matches yours:

- **Encrypting individual files inside the repository** — the natural fit
  when the remote host must stay readable, for example while collaborating
  through a public git host.
- **An encrypted volume holding a bare repository** — natural when you
  already work with encrypted volumes and don't mind mounting one before
  every push.
- **A general backup tool archiving the repository** (restic, Borg,
  Kopia) — natural when one tool already backs up everything else on the
  machine. Its artifact is an archive restored with that same tool;
  Coffer's artifact is a remote you clone like any other.
- **An encrypted remote helper** — the whole remote is ciphertext; `git
  push` stays a single step, on every platform, with no drivers or
  daemons. For the offsite copy, this turns the backup job itself into
  `git push` — no plaintext copy is ever written. Two honest boundaries:
  a vault stores git data (commits, branches, tags), not a forge's
  issues, wikis, or CI records — keep the platform's own dump for those.

Coffer lives in the fourth shape. Its design follows the path that
[git-remote-gcrypt](https://github.com/spwhitton/git-remote-gcrypt) proved
on Linux, and aims to offer it everywhere with no dependencies — we are
grateful for the ground it broke.

## How Coffer is tested

Claims are cheap; here is what stands behind this one. Everything below
is reproducible from this repository's CI logs, release archives, and
documents — including what was *not* tested by hand.

- **Every push runs the full suite** on Linux, Windows, and macOS: unit
  tests; end-to-end runs against real git (clone, push, incremental
  updates, forced overwrites, deletions, `--atomic`); crash injection —
  the helper is killed mid-push and the vault must recover; and a
  corruption matrix that mangles every part of the vault in turn and
  requires a loud failure, never silent damage. On top of that: the race
  detector, `govulncheck`, fuzz seeds for every on-media parser,
  cross-compilation of all six release targets — and a dedicated leg
  that builds **git 2.30.0 from source** and runs the entire suite
  against it, so the supported floor is exercised on every push rather
  than asserted.
- **Review rounds.** The cryptography, the Go application security, and
  the git protocol and backup-integrity behavior were each audited in
  cold-start rounds by agent reviewers with no shared context, a
  darwin-focused specialist pass covered the macOS surface, and each
  release candidate's diff got its own adversarial review. Every
  finding became a fix with a regression test before the next
  candidate. These are agent-run reviews, not a paid third-party audit.
- **Seven release candidates, device-tested where it counts.** Each
  rc's release notes say exactly what it fixed; from rc.4 on, every one
  was installed from its published archives and pushed through the full
  functional checklist on real external media — Windows (NTFS, exFAT,
  FAT32) and Linux (FAT32, exFAT) — including cross-system relay, where
  one operating system writes a vault and another reads it back. macOS
  was never hand-tested on Apple hardware: its coverage is the CI
  macOS leg (the full suite, on every push) plus the specialist
  darwin review — no device claims are made for it. **v1.0.0 carries
  rc.7's code unchanged; only documentation differs.**
- **The format is pinned, not promised.** The on-disk format has a
  normative specification and public AEAD test vectors, so an
  independent implementation — or a future version — can verify a vault
  byte-for-byte. And the exit path is built in: `gitcoffer export-bundle`
  writes a plain git bundle any stock git can clone.

Where the boundaries are — LFS content, metadata visibility, what a
tampering medium-writer can and cannot do —
[docs/threat-model.md](docs/threat-model.md) and
[docs/support-matrix.md](docs/support-matrix.md) say so in plain text.

## Documentation

| Document | Purpose |
|---|---|
| [User guide](docs/user-guide.md) | install, everyday use, keys and recovery, maintenance |
| [Architecture](docs/architecture.md) | design decisions, components, protocol flows |
| [Vault format specification](docs/format-spec.md) | normative on-disk format — build against this |
| [Support matrix](docs/support-matrix.md) | tested configurations and explicit boundaries |
| [Threat model](docs/threat-model.md) | what Coffer does and does not protect |
| [Development plan](docs/development.md) | stack, phases, testing strategy, conventions |
| [Contributing](CONTRIBUTING.md) | repository rules and how to contribute |

## Contributing

Issues and pull requests are welcome — holes in the threat model, gaps in
the guide, portability reports, and fixes all count. All artifacts in this
repository are written in English; see
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
