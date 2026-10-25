# User Guide

Coffer turns a directory on an external or secondary disk into an encrypted
git remote. `git push` works exactly as always; the bytes that reach the
disk are ciphertext. This guide covers installation, everyday use, key
management, recovery on a new machine, and maintenance.

## Requirements

- git ≥ 2.30 on `PATH`
- The two GitCoffer binaries, `git-remote-coffer` and `gitcoffer`, on `PATH`
  (git discovers the helper through `PATH`)
- A vault on a *different physical disk* than your working copy — that is
  the point: one disk failure must not destroy both

## Install

**Download a release** (Linux, Windows, macOS; amd64 and arm64) from the
[releases page](https://github.com/Ziqing7226/GitCoffer/releases), unpack, and
put both binaries on `PATH`. Verify with:

```console
$ gitcoffer version
gitcoffer v1.0.0
$ git-remote-coffer          # run with no arguments, prints its usage note;
                             # normally git invokes it for you
```

On Windows, the PowerShell installer
(`scripts/install.ps1`) does the same through `Invoke-WebRequest`.

**Or install with the script** (Linux/macOS): downloads the latest
release, verifies the checksum, installs into `~/.local/bin` — see the
script header for options:

```console
$ curl -fsSL https://raw.githubusercontent.com/Ziqing7226/GitCoffer/main/scripts/install.sh | sh
```

**On macOS**, two platform specifics:

- **Terminal vs GUI apps.** VSCode launched from the Dock inherits
  launchd's minimal `PATH` (`/usr/bin:/bin:/usr/sbin:/sbin`) and will
  not find the binaries in `~/.local/bin` — pushes fail with
  helper-not-found while Terminal works. Fix either way: launch `code`
  from a terminal, run
  `launchctl setenv PATH "$PATH"` before starting VSCode, or install
  into `/usr/local/bin` (needs sudo once).
- **Gatekeeper.** Archives downloaded with a **browser** carry the
  `com.apple.quarantine` flag; first execution is blocked. Recover
  once: `xattr -d com.apple.quarantine <binary>` on both binaries, or
  allow via System Settings → Privacy & Security. The `curl`/`wget`
  installer path adds no quarantine.

The version line is self-describing: release archives print the
release version, `go install …@vX` prints that exact version, and a
repository build reports the commit it was built from.

**Build from source** with Go ≥ 1.27:

```console
$ go install github.com/Ziqing7226/GitCoffer/cmd/gitcoffer@v1.0.0
$ go install github.com/Ziqing7226/GitCoffer/cmd/git-remote-coffer@v1.0.0
```

**Package managers.** Homebrew tap (macOS / Linux):
`brew tap Ziqing7226/gitcoffer` then
`brew install Ziqing7226/gitcoffer/gitcoffer`. scoop (Windows):
`scoop bucket add ziqing7226 https://github.com/Ziqing7226/scoop-bucket`
then `scoop install gitcoffer`. A winget entry
(`winget install Ziqing7226.GitCoffer`) is in review.

**Windows note:** add the install directory to `PATH` via system settings;
no administrator rights are needed anywhere.

**Shell completion.** `gitcoffer completion` prints a tab-completion
script for four shells. Load it once — `gitcoffer` subcommands and flags
then complete as you type:

| Shell | One-time setup |
|---|---|
| bash | `eval "$(gitcoffer completion bash)"` in `~/.bashrc` |
| zsh | `eval "$(gitcoffer completion zsh)"` in `~/.zshrc` — the script loads the completion system itself if the shell has not |
| fish | `gitcoffer completion fish > ~/.config/fish/completions/gitcoffer.fish` |
| PowerShell 7+ | `gitcoffer completion powershell >> $PROFILE` — native-command completion needs pwsh; Windows PowerShell 5.1 is not supported |

## Quick start

Pick the vault path for your platform — Linux `/mnt/usb/…`,
macOS `/Volumes/<volume>/…`, Windows `D:\backups\…` — and substitute it
in the commands below (shown with the Linux path). That Linux path is a
placeholder: on desktop Linux a removable drive usually mounts under
`/run/media/<you>/<drive-label>`, and `/mnt` itself is root-owned —
`lsblk` or `ls /run/media/$USER` lists the real mount points:

```console
$ gitcoffer init /mnt/usb/myproject.coffer
Enter passphrase for the new vault: ********
Repeat passphrase: ********
Vault created: /mnt/usb/myproject.coffer

$ cd myproject
$ git remote add origin coffer::/mnt/usb/myproject.coffer
$ git push -u origin main
```

From then on, `git pull`, `git fetch`, `git clone`, and VSCode's Sync
button all work against the vault. The passphrase is requested through
git's own credential flow once per operation — as a terminal prompt, as a
native VSCode input box, or silently from a credential helper you have
configured.

On another machine that knows the passphrase:

```console
$ git clone coffer::/mnt/usb/myproject.coffer
```

Everything — every branch, tag, and commit — is reconstructed from the
vault alone. A git repository is ~40 MB of history? The vault is a
complete backup, not a mirror of your working files.

### Multiple projects: one vault each

A vault is one git repository, exactly like one URL on a public host is
one repository. Two projects pushing `main` into the same vault share
that ref — the second push moves or (with `--force`) overwrites the
first, because that is what pushing two `main`s to one repository
means; git refuses non-fast-forward updates without `--force`, so the
protection is the same as on any remote. Keep one project per vault;
if you deliberately share a vault, give each project its own branch
name (`repo1-main`, `repo2-main`) — everything else (fsck, gc, keys)
treats the vault as a whole.

### Two remotes at once

A vault does not have to replace your host. Keep `origin` on GitHub and
add the vault as a second remote:

```console
$ git remote add vault coffer::/mnt/usb/myproject.coffer
$ git push vault --all
$ git push vault --tags
```

From then on, `git push` — and VSCode's Sync button — still goes to
`origin`, because the branch tracks it; `git push vault` updates the
backup. Share on the host, back up on the drive, from the same clone.

### Passphrases on Windows

Interactive use needs nothing special: git prompts in the terminal, and
VSCode shows its native input box. For scripted use, prefer git's
credential system over GIT_ASKPASS — askpass scripts are awkward in
cmd/batch (a `.bat` `echo` ends lines with CRLF, and any layer that
passes the carriage return into the passphrase breaks authentication;
GitCoffer trims it defensively, but not every tool in the chain does):

```console
$ git config credential.helper store        # or: manager (Credential Manager)
$ printf "protocol=coffer\nhost=coffer\npath=<vault id>\nusername=coffer\npassword=<passphrase>\n\n" | git credential approve
```

The `<vault id>` is printed by `gitcoffer init` when the vault is
created, and again in the header of `gitcoffer status`.

**Why a push may ask for the passphrase twice.** git runs the helper in
two separate stages — `list for-push` (read the vault's refs), then the
actual `push` — and without a configured credential helper nothing is
remembered between them, so both stages prompt. Configure one and you
are asked once per window:

```console
$ git config --global credential.helper 'cache --timeout=3600'  # in-memory, 1 h
$ git config --global credential.helper store                   # plaintext file
```

A `rekey` makes any cached passphrase stale — see the reject command
above.

(In PowerShell, pipe a double-quoted string with `` `n `` line breaks
instead of printf.)

After a successful push the passphrase is approved to git, so a helper
like Windows Credential Manager may cache it. That is the feature
working — but after `gitcoffer rekey` you must evict the cached value
before the next push, or the old passphrase keeps answering (the
authentication-failure error names this fix):

```console
$ printf "protocol=coffer\nhost=coffer\npath=<vault id>\nusername=coffer\n\n" | git credential reject
```

### Nicer remote URLs

Long vault paths — especially with spaces in them — make for clumsy
remotes. Git has a built-in answer, URL rewriting: one config line tells
git that any URL starting with `usb://` really means `coffer::` plus a
vault directory, and from then on remotes can use the short form.

```console
$ git config --global url."coffer::/mnt/usb/".insteadOf "usb://"
$ git remote add origin usb://myproject.coffer
```

The rule lives only in your git config — git rewrites the URL before
every use, so every client resolves `usb://myproject.coffer` to the vault
behind it. Two things to know: the rule is per user and per machine, so a
colleague or a new machine needs the same line; and when the drive's
mount path changes — a new drive letter on Windows, a new mount point on
Linux — updating this one rule fixes every clone at once, with no
`git remote set-url` rounds.

## How it works (one paragraph)

Coffer is a standard
[git remote helper](https://git-scm.com/docs/gitremote-helpers): when git
sees a `coffer::<path>` URL it runs `git-remote-coffer`, which reads the
pushed objects straight from your repository, encrypts them
(Argon2id key derivation, XChaCha20-Poly1305 AEAD), and stores them in the
vault directory; on fetch it does the reverse. Git never notices the
difference, which is why every git client — including VSCode — works
unchanged. The on-disk format is specified and frozen in
[format-spec.md](format-spec.md).

## The coffer CLI

| Command | Purpose |
|---|---|
| `gitcoffer init <dir>` | create a new vault (prompts for a new passphrase) |
| `gitcoffer status <dir>` | inspect a vault: format, key slots (passphrase / key-file entries), refs, packs |
| `gitcoffer rekey <dir>` | change the passphrase of the slot it opens |
| `gitcoffer key add [-keyfile <path>] <dir>` | add a passphrase slot, optionally requiring a key file as a second factor (the flag goes before the directory) |
| `gitcoffer key remove <dir> <id>` | remove a key slot (never the last one) |
| `gitcoffer key list <dir>` | list key slots (no passphrase needed) |
| `gitcoffer gc [--prune] <dir>` | report orphaned objects, temp files, old generations — `--prune` actually removes them |
| `gitcoffer fsck <dir>` | verify every structure of the vault |
| `gitcoffer doctor <dir>` | check the environment and the vault, and report |
| `gitcoffer check <dir>` | probe a vault from a script (exit 0 opens, 1 does not open, 100 not a vault) |
| `gitcoffer move <dir> <new-path>` | relocate a vault directory (same-disk moves are atomic; copies are verified before success) |
| `gitcoffer migrate <dir>` | convert a v1 vault to format v2 in place (one-way: binaries older than 1.1.0 then refuse it) |
| `gitcoffer export-bundle <dir> <file>` | export the vault as a plain git bundle |
| `gitcoffer version [--json]` | print the build version (optionally as JSON) |
| `gitcoffer completion <shell>` | print a completion script for bash, zsh, fish, or powershell |
| `gitcoffer help [<command>]` | print detailed usage for one command, or for all of them |

Passphrase prompts read from the terminal (hidden); when stdin is not a
terminal — scripts, CI — each prompt reads one line, so every subcommand
is scriptable.

## Keys and recovery

**There is no passphrase recovery, by design.** The passphrase (plus a
key file, if that slot requires one) is the only way in. A key file adds
a second factor; losing it locks out every slot that requires it.

- Rotate your passphrase on a schedule: `gitcoffer rekey` rewrites only the
  tiny `vault.meta` — object data is never re-encrypted, so it is fast at
  any vault size.
- A rekey makes cached credentials stale. If pushes suddenly fail with
  *authentication failed* after a rekey, clear the cached value:
  `printf 'protocol=coffer\nhost=coffer\npath=<vault id>\n\n' | git credential reject`
- Share access with a collaborator or machine by adding a slot
  (`gitcoffer key add`) instead of sharing one passphrase.
- A key-file slot records the key file's **exact path** and reads that
  path first. A copy of the same bytes elsewhere does not unlock the slot
  at that path, and a file whose content *changed* at the recorded path
  is refused with a precise error. When the recorded path no longer
  exists — the vault and the key file moved together to another machine
  or drive letter — the slot records what the key file *hashes to*, and
  Coffer recognizes the file by content: a shallow search of the folder
  holding the vault and the vault directory itself. Put the key file
  next to the vault on the medium and the pair travels freely; keep it
  elsewhere (the stronger setup — a thief of the medium then misses the
  second factor) and that path must exist on every machine that opens
  the vault. `gitcoffer doctor` reports a recognized key file and
  suggests re-registering it with `gitcoffer key add`.

**Recovering on a fresh machine:** install git and Coffer, mount the
medium, `git clone coffer::<path>`, enter the passphrase. Keeping a copy
of the release archive next to the vault is a convenience for offline
bootstrapping, never a dependency.

## Leaving, and being sure you can

`gitcoffer export-bundle <vault> <file.bundle>` decrypts the vault into a
standard git bundle that stock git alone can clone or verify — the exit
path needs neither Coffer nor the vault format. The bundle file itself
is plaintext, so store it accordingly. The bundle is a point-in-time
snapshot: a push that lands while the export runs is simply not in it
(export never blocks writers). `gitcoffer doctor <vault>` gives a
one-shot health report of the environment and the vault; boundaries of
what is supported (LFS content, submodules, sha256, shallow clones,
FAT32 file-size caps) are listed in
[support-matrix.md](support-matrix.md).

## Maintenance

- `gitcoffer fsck` verifies every AEAD seal, the manifest chain, and every
  object's checksum; run it when a medium had a rough day — and on a
  schedule anyway: after big pushes, or once a month, to catch a failing
  medium early. It reports the
  first divergence per structure and never attempts recovery. When it
  reports an unreadable manifest generation, re-push from the original
  repository anything that was acknowledged just before the failure —
  the damaged generation is quarantined by the next push, and
  `gitcoffer gc --prune` would otherwise reclaim its objects.
- `gitcoffer gc` reports reclaimable space from interrupted pushes
  (orphaned object files), leftover temp files, and old manifest
  generations; `--prune` performs the deletion. It refuses to delete
  anything if any manifest generation fails to decrypt. Running gc
  without flags reports only; `--prune` deletes. One honest caveat: the
  vault is a remote, not an immutable archive — and plain gc reclaims
  only crash debris, because an object file written by a push that also
  carried a since-deleted branch stays referenced as long as any object
  in it is reachable.
- `gitcoffer gc --repack` reclaims deleted history for real: every
  object file is decrypted, git rebuilds one pack from the current
  refs' history, and the vault keeps exactly that. Objects under
  deleted refs physically leave the medium — when that matters (a
  secret committed by mistake), this is the command. The trade: the
  recovery manifest generations are dropped along with the old packs,
  and until you repack, treat deleted-but-once-pushed history as still
  on the disk, encrypted. An interrupted repack leaves a vault that
  opens; the next `gc --prune` (or another repack) sweeps the leftovers.
- Both commands serialize with writers through the vault lock; concurrent
  pushes queue safely rather than corrupting.

### Checking a vault on a schedule

`gitcoffer check <vault-directory>` answers one question — does the
vault open with its passphrase? — and encodes the answer in its exit
code, so scripts and schedulers can branch on it:

| Exit code | Meaning |
|---|---|
| 0 | the vault opened |
| 1 | the path holds a vault that does not open — wrong passphrase, or damage |
| 100 | not a vault, or the path is unreachable (a drive can be unplugged) |

`check` proves only that the vault opens. Run `gitcoffer fsck` for full
verification; a schedule that runs both catches a failing medium early.

A scheduled run has no keyboard, but the check needs the passphrase.
Put it in a file and feed that file on stdin. Protect the file with
permissions: an attacker who can read it can open the vault. Keeping it
on the machine that already holds the plaintext repository grants an
attacker nothing new; do not copy it onto the vault drive itself.

Store it once. Linux and macOS:

```bash
umask 077
printf '%s\n' 'your-passphrase' > ~/.coffer-pass
```

Windows (PowerShell):

```powershell
"your-passphrase" | Out-File -Encoding ascii $env:USERPROFILE\.coffer-pass
```

Then schedule the checks — a nightly `check` and a monthly `fsck`. On
Linux and macOS (`crontab -e`); a silent run is a passing run, and cron
mails you whatever a failing or warning run prints:

```cron
30 3 * * * gitcoffer check "/mnt/usb/myproject.coffer" < "$HOME/.coffer-pass" >/dev/null || echo "coffer check FAILED: myproject"
0 4 1 * * gitcoffer fsck "/mnt/usb/myproject.coffer" < "$HOME/.coffer-pass" >/dev/null || echo "coffer fsck FAILED: myproject"
```

On Windows, Task Scheduler records the exit code as the task's Last
Run Result (0 = the vault opened):

```bat
schtasks /create /tn "Coffer check" /sc daily /st 03:30 /tr "powershell -NoProfile -Command ""Get-Content $env:USERPROFILE\.coffer-pass | gitcoffer check D:\backups\myproject.coffer"""
```

### Moving a vault

A vault is a self-contained directory, so moving it to another disk or
folder is safe whenever no push is running:

```bash
gitcoffer move "/mnt/usb/myproject.coffer" "/mnt/usb2/myproject.coffer"   # Linux
gitcoffer move "/Volumes/Backup/myproject.coffer" "/Volumes/New/myproject.coffer"   # macOS
gitcoffer move D:\backups\myproject.coffer E:\myproject.coffer            # Windows
```

On one filesystem the move is a single atomic rename. Across filesystems
the vault is copied while writers are frozen out, and the copy is opened
and fscked before the command reports success — only then are you asked
whether to delete the source (answering no, or running from a script,
keeps it as the natural backup). The source is never modified along the
way.

Afterwards every clone needs one line — the command prints it for you:

```bash
git remote set-url origin "coffer::/mnt/usb2/myproject.coffer"
```

One caveat: a key-file slot records the key file's exact path. If the
key file lived inside the moved vault (or its recorded path was
relative), `move` flags the slot — content recognition usually finds the
file at the new location anyway; otherwise restore a key file at the
recorded path, or replace the slot with `gitcoffer key add`.
Passphrase-only vaults move without any caveat.

Host↔vault relocations are a different operation (moving a project to or
from GitHub): those are the plain-git migration steps in the README, not
this command.

### Upgrading a vault to format v2

Format v2 changes one thing: each manifest's authentication is bound to
its generation number, so a rolled-back manifest (an older generation's
bytes re-filed under a newer generation's name) is rejected instead of
warned about. Everything else — structures, key slots, object files — is
identical. New vaults are created on v2 directly; `migrate` is for
vaults made by older versions:

```bash
gitcoffer migrate "/mnt/usb/myproject.coffer"
```

The command asks for confirmation; a script passes `-yes`. fsck must
pass first, and the vault never leaves its directory: only vault.meta
and the manifest generations are rewritten — object data is untouched,
disk usage does not double, clones keep their URLs. An interrupted
migration is finished by running the command again.

The upgrade is one-way: binaries older than 1.1.0 — including 1.0.0 —
refuse a v2 vault with an explicit version error. On a drive you hand to
another machine, migrate only when that machine runs GitCoffer 1.1.0 or
newer. `status` and `doctor` print a hint on v1 vaults.

## Troubleshooting

| Symptom | Meaning and fix |
|---|---|
| `not a coffer vault` on push | The remote URL does not point at a vault. Check the path, or run `gitcoffer init`. |
| `authentication failed` on every operation | Wrong passphrase, or a credential helper answers with a stale value (common right after a rekey). Evict it with the `git credential reject` line above. |
| `key file ... no such file` | A second-factor slot's key file is missing at its recorded path. If the file moved with the vault, Coffer recognizes it by content in the folder holding the vault — otherwise restore it at the recorded path, or add a new slot with the file at its new location. |
| `key file ... is not a regular file` | The recorded path is a symlink, device, or oversized (>1 MiB) file. Coffer refuses such paths because they come from on-media metadata — point the slot at the real file instead. |
| Push refused: repository is shallow | The push came from a depth-limited clone, whose history is truncated. Run `git fetch --unshallow` against its current origin and push again. |
| Push or fetch refused: repository uses sha256 object ids | The vault format speaks sha1 object ids. Re-create the repository with the default sha1 format (`git init` without `--object-format=sha256`). |
| LFS repository: push fails with `batch request: missing protocol` | Expected: git-lfs does not support `coffer::` remotes. The vault stores only the LFS pointer files (the push prints a warning). Remove the lfs pre-push hook if you want the pointers in the vault, and back the LFS objects up separately. |
| Push refused: newest manifest generation unreadable | The newest vault index is damaged; an older state is being served. Run `gitcoffer fsck`, then simply push again — the damaged generation is quarantined automatically. Re-push from the original repository anything that reported ok just before the failure (gc --prune would otherwise reclaim it). |
| Push refused: the vault inventory has grown beyond the manifest limit | The vault index is capped at 8 MiB (roughly 150–200k stored objects) and rewrites wholesale on every push; very large repositories hit this ceiling. The vault is intact — see the support matrix's scale boundary. |
| Push refused: non-fast-forward | The vault's branch would be overwritten by unrelated or rewritten history. Fetch and merge first; `--force` overwrites deliberately — the overwritten commits become unrecoverable after `gitcoffer gc --prune`. |
| `git pull` does nothing after `origin` was switched to the vault; VSCode shows "Publish Branch" | The branch still tracks the old remote — standard git behavior, not a helper fault. The helper attaches the branch by itself after a push, or after a fetch that transfers something (it prints what it did). When nothing was transferred, attach once by hand: `git branch --set-upstream-to=origin/<branch> <branch>`. If the local branch name differs from the vault's, `git checkout -B <vault-branch> origin/<vault-branch>` instead. |
| `another coffer operation is writing` | A concurrent writer holds the vault lock. On Windows, a crashed holder's lock is stolen as soon as its pid is detected dead; on any platform, if you are certain none is running you can delete `<vault>/vault.lock` directly. |
| Clone of a non-`main` repository checks out an empty tree | Fixed long before 1.0.0: the alphabetically first branch is advertised as HEAD. If you still see this, update both binaries to the current release. |

## Security notes

The threat model, in full, is [threat-model.md](threat-model.md). In
short: the medium at rest reveals only file count, approximate sizes, and
timestamps — never content, file names, or ref names. Coffer does not
protect a host that is compromised *while* the vault is open, and it
provides no deniability. The format is frozen and pinned by published
[test vectors](test-vectors.json).
