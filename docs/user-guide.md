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
| `gitcoffer export-bundle <dir> <file>` | export the vault as a plain git bundle |
| `gitcoffer version [--json]` | print the build version (optionally as JSON) |
| `gitcoffer completion <shell>` | print a completion script for bash, zsh, fish, or powershell |

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
- A key-file slot records the key file's **exact path** and always reads
  that path: a copy of the same file elsewhere does not unlock the slot,
  and moving the vault or the key file to another machine (or another
  drive letter) locks the slot until the file is back at the recorded
  path — or a new slot is added with the file at its new location. Keep
  key files on a stable path that exists on every machine that opens
  the vault.

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
  without flags reports only; `--prune` deletes. Two honest caveats:
  the vault is a remote, not an immutable archive — and today gc
  reclaims only crash debris: objects committed under a ref you later
  deleted stay on the medium (encrypted, unreferenced) until a future
  repack feature reclaims them. Treat deleted-but-once-pushed history
  as still on the disk.
- Both commands serialize with writers through the vault lock; concurrent
  pushes queue safely rather than corrupting.

## Troubleshooting

| Symptom | Meaning and fix |
|---|---|
| `not a coffer vault` on push | The remote URL does not point at a vault. Check the path, or run `gitcoffer init`. |
| `authentication failed` on every operation | Wrong passphrase, or a credential helper answers with a stale value (common right after a rekey). Evict it with the `git credential reject` line above. |
| `key file ... no such file` | A second-factor slot's key file is missing at its recorded path. Restore it; nothing else will unlock that slot. |
| `key file ... is not a regular file` | The recorded path is a symlink, device, or oversized (>1 MiB) file. Coffer refuses such paths because they come from on-media metadata — point the slot at the real file instead. |
| Push refused: repository is shallow | The push came from a depth-limited clone, whose history is truncated. Run `git fetch --unshallow` against its current origin and push again. |
| Push or fetch refused: repository uses sha256 object ids | The vault format speaks sha1 object ids. Re-create the repository with the default sha1 format (`git init` without `--object-format=sha256`). |
| LFS repository: push fails with `batch request: missing protocol` | Expected: git-lfs does not support `coffer::` remotes. The vault stores only the LFS pointer files (the push prints a warning). Remove the lfs pre-push hook if you want the pointers in the vault, and back the LFS objects up separately. |
| Push refused: newest manifest generation unreadable | The newest vault index is damaged; an older state is being served. Run `gitcoffer fsck`, then simply push again — the damaged generation is quarantined automatically. Re-push from the original repository anything that reported ok just before the failure (gc --prune would otherwise reclaim it). |
| Push refused: the vault inventory has grown beyond the manifest limit | The vault index is capped at 8 MiB (roughly 150–200k stored objects) and rewrites wholesale on every push; very large repositories hit this ceiling. The vault is intact — see the support matrix's scale boundary. |
| Push refused: non-fast-forward | The vault's branch would be overwritten by unrelated or rewritten history. Fetch and merge first; `--force` overwrites deliberately — the overwritten commits become unrecoverable after `gitcoffer gc --prune`. |
| `git pull` does nothing after `origin` was switched to the vault; VSCode shows "Publish Branch" | The branch still tracks the old remote — standard git behavior, not a helper fault. Run `git fetch origin`, then `git branch --set-upstream-to=origin/<branch> <branch>`. If the local branch name differs from the vault's, `git checkout -B <vault-branch> origin/<vault-branch>` instead. |
| `another coffer operation is writing` | A concurrent writer holds the vault lock. On Windows, a crashed holder's lock is stolen as soon as its pid is detected dead; on any platform, if you are certain none is running you can delete `<vault>/vault.lock` directly. |
| Clone of a non-`main` repository checks out an empty tree | Fixed long before 1.0.0: the alphabetically first branch is advertised as HEAD. If you still see this, update both binaries to the current release. |

## Security notes

The threat model, in full, is [threat-model.md](threat-model.md). In
short: the medium at rest reveals only file count, approximate sizes, and
timestamps — never content, file names, or ref names. Coffer does not
protect a host that is compromised *while* the vault is open, and it
provides no deniability. The format is frozen and pinned by published
[test vectors](test-vectors.json).
