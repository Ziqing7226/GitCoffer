# Changelog

## Unreleased

- CLI: `gitcoffer help` (also `-h`/`--help`, and `gitcoffer help
  <command>`) now prints a self-describing guide — what the tool is, the
  `coffer::` URL form, every command with its arguments and behavior,
  how passphrase prompts behave under a non-terminal stdin, and the
  exit codes — so a person or a program can operate the CLI without
  opening the documentation. Usage errors exit 2 and per-command usage
  lines point at the matching help section; `-h` on a subcommand exits
  0 instead of reporting a flag error.
- CLI: `gitcoffer check <vault-directory>` — the three-state probe from
  the 1.1.0 plan, in the style of git-remote-gcrypt's --check: exit 0
  the vault opens with the passphrase, 1 the path holds a vault that
  does not open (wrong passphrase or damage), 100 not a vault or
  unreachable. Serving an older manifest generation prints a warning
  but still exits 0 — a recoverable vault must not read as missing. The
  user guide's Maintenance section gained the scheduling recipe: cron
  and Task Scheduler lines that feed the passphrase from a
  permission-protected file, with the honest note about what that file
  grants.
- Helper: upstream auto-attach, from a real v1.0.0 report (after
  re-pointing `origin` from a host to a vault, `git pull` silently did
  nothing and VSCode showed "Publish Branch" — no upstream tracking
  after the switch). After a push, or after a fetch that transfers
  something, when the caller's current branch tracks nothing anywhere and
  the coffer remote has a branch of the same name, the helper attaches
  it (`main` -> `origin/main`) and prints what it did plus the undo
  (`git branch --unset-upstream`). Guards, each with a reason: current
  branch only and only when the push was exactly that branch — never
  under `--all`/`--mirror`, where "same name exists" carries no intent;
  never when any upstream is configured, so the documented
  host-plus-vault dual-remote flow keeps its tracking; never under a dry
  run; non-bare callers only; a clone mid-fetch is left to git's own
  tracking setup. When a guard blocks an otherwise-eligible attach, the
  helper prints the `git branch --set-upstream-to` hint instead of
  acting. The README's "pull does nothing" section and the guide's
  troubleshooting row now lead with the automatic fix.
- CLI: `gitcoffer move <vault-directory> <new-path>` — relocate a vault
  directory, from the 1.1.0 plan. The source is never modified, so the
  worst outcome is a cleaned-up partial target. Refusals: source not a
  vault or actively locked; target non-empty, an existing vault, or
  inside the source. Same-filesystem moves are one atomic rename;
  cross-filesystem moves copy under the source's writer lock and the
  copy is opened and fscked before success is reported, then the
  command asks whether to remove the source (declining, or a
  non-interactive run, keeps it as the natural backup). The report
  prints the exact `git remote set-url` line, flags key-file slots
  whose recorded path the move invalidates, and cautions when the
  target sits on an online or synced filesystem. The guide's
  Maintenance section documents the whole flow, including the key-file
  caveat.
- Format v2: the manifest AAD additionally binds the generation number
  ("coffer/2/manifest/<id>/<generation>"), so refiling an older
  manifest's ciphertext under a newer generation's name now fails
  authentication — rollback becomes a rejection instead of a fsck-time
  warning. Structures and slot crypto are unchanged from v1; a v2
  manifest read tolerates v1-sealed generations so an interrupted
  migration stays readable (meta flips to v2 first, then each
  generation is resealed; the prev chain hashes the plaintext, so
  resealing leaves it intact). New vaults are created on v2. 1.0.0
  binaries refuse a v2 vault at vault.meta with the explicit
  unsupported-version error — the one-way door the migration command
  states before acting. The grammar leaves room for the recorded
  format-v2 candidates (unknown slot input types and manifest fields
  are skipped by this build's reader) so public-key slots can land
  without a v3.
- CLI: `gitcoffer migrate [-yes] <vault-directory>` — the explicit,
  in-place v1→v2 conversion: fsck must pass first, the writer lock is
  held throughout, only meta and manifests are rewritten with the same
  write-temp-and-rename atomicity rekey uses, the result is reopened
  and fscked before success is reported, and an interrupted run is
  finished by running it again. Confirmation is required (or `-yes`
  for scripts); `status` and `doctor` print an upgrade hint on v1
  vaults. Conformance vectors pin the v2 manifest seal.
- Key files: migratable paths via content recognition. A key-file slot
  now records what its key file hashes to (`keyfile_id`, an optional
  field in the same version-free class as `input`/`keyfile`, so 1.0.0
  readers simply ignore it). When the recorded path no longer exists —
  the vault-plus-key-file set moved between machines or drive letters —
  the file is recognized by content through a shallow, bounded search of
  the folder holding the vault and the vault directory itself. A file
  whose content changed at the recorded path is refused with a precise
  error instead of a generic authentication failure. `rekey` refreshes
  the recorded hash; `doctor` reports a recognized key file and suggests
  re-registering it; `move`'s key-file notes now mention recognition.
  The threat model states the honest trade: the hash is plaintext in
  vault.meta, letting an attacker verify key-file candidates without
  the Argon2id cost — random key files (what Coffer creates) are
  unaffected.
- Versioning convention: a.b.c with positive integers — a is the major
  direction, b the minor direction (its goals are written down before
  work starts), c the rc counter of the current direction; candidates
  (c > 0) ship as pre-releases, stables (c = 0) as full releases. The
  contract lives in CONTRIBUTING.md and the release workflow now marks
  c > 0 tags as pre-releases.
- README: a second screenshot set — "From GitHub to a vault", placed
  with the migration how-to — a single maximized-terminal session against a real
  GitHub-cloned repository (the session starts at git remote -v
  showing the GitHub origin): gitcoffer init, the set-url that moves
  origin to the vault, a push with the helper's own progress lines,
  then the vault on disk — a plain folder, and a file manager
  showing git.coffer among the drive's other folders. Captured with
  editor AI features disabled and an English locale.
- README: an "Inside VSCode" section shows the native-editor experience
  — publish, the passphrase prompt, in-sync state, and the built-in
  commit graph — as four screenshots taken against a real vault on a
  USB drive.
- development.md: the maintainer named the 1.1.0 work plan — a
  three-state vault check command, migratable key-file paths, gc
  repacking, and the format-v2 manifest-generation AAD binding — and
  declined the VSCode companion extension outright.
- Documentation consistency round: architecture's CLI list includes
  `completion`; the user guide documents the key-file registered-path
  model (exact path required, copies do not unlock, moving the vault or
  key file to another machine locks the slot); AGENTS states the tap
  and bucket are live with winget in review; a stale troubleshooting
  row reworded.
- Documentation: the weakest-link scenario merged into "Choosing an
  approach"; "How Coffer is tested" now states plainly that the review
  rounds are agent-run (not a paid third-party audit) and that macOS
  coverage is CI plus specialist review with no hand-testing on Apple
  hardware; the install sections name the live Homebrew tap and scoop
  bucket (winget in review); .gitignore gains coverage and log
  artifacts.

## 1.0.0 (2026-10-12)

- macOS review of the rc.3→rc.6 delta — all findings fixed: the
  key-file containment check compares directory identity along the
  recorded path, not just path spelling (on case-insensitive volumes —
  default APFS, exFAT/FAT media — a differently-cased path could
  previously place a second factor inside the vault it protects); the
  zsh completion loads compinit itself when the stock shell has not
  (the eval path previously printed "command not found: compdef" and
  activated nothing there); doctor's key-file presence probe reads the
  recorded path verbatim instead of its display-sanitized spelling;
  fsck no longer counts Finder metadata junk in obj/ as orphans.
- Windows: `gitcoffer doctor` now reports the filesystem (NTFS, exFAT,
  FAT32, ...) and warns on FAT media like on Linux and macOS — the
  filesystem line had never been implemented on Windows. rc.5's exFAT
  fix note was Linux-side only: the corrected statfs magic does not
  compile into Windows builds, and the note should have said so.
- macOS: GUI-app PATH guidance (Dock-launched VSCode does not see
  ~/.local/bin), Gatekeeper quarantine recovery for browser downloads,
  a zsh completion that activates via eval on the default shell, bash
  completion falls back to file names, exFAT no longer triggers the
  FAT32 size-cap warning, AppleDouble/.DS_Store files no longer count
  as object files, and object-file reads refuse planted symlinks and
  FIFOs like every other untrusted read.
- CLI shape finalized before 1.0.0: `gitcoffer gc` is report-only by
  default and deletion requires `--prune` (the `--dry-run` flag is
  gone — the default IS the dry run). `gitcoffer version --json`
  serves structured consumers, and `gitcoffer completion` emits
  scripts for bash, zsh, fish, and powershell. Installers
  (`scripts/install.sh`, `scripts/install.ps1`) download, verify, and
  install release binaries in one step.

- Reliability and cross-platform review round — all findings fixed:
  export-bundle resolves its output path against the caller's directory
  (a relative path previously vanished with the scratch repository);
  a vault can no longer be bricked by adding a 17th key slot or by a
  manifest growing past the 8 MiB read ceiling (both refuse honestly
  with the vault intact); force-with-lease performs its sanctioned
  non-fast-forward overwrites (`option cas`); rekeying a key-file slot
  preserves the second factor instead of silently downgrading it;
  a ref deleted and re-created in one push batch requires --force;
  passphrases containing control characters are refused at creation;
  dead-pid detection distinguishes reaped children (the initial
  EPERM-as-alive hardening broke Unix lock stealing); untrusted paths
  are sanitized before printing.
- exFAT detection fixed: the kernel statfs magic in doctor was wrong,
  so every real exFAT disk reported an unknown filesystem (found
  testing on real external drives).
- Final pre-1.0.0 security review (cold, full code) — all findings
  fixed: manifest reads are bounded and symlink-refusing like every
  other metadata read (a planted manifest.<n> symlink/FIFO/oversized
  file previously hung or OOM'd every operation); concurrent key
  operations re-read vault.meta under the lock so a rotation can no
  longer be silently undone by a stale snapshot; the helper surfaces
  fallback-to-older-generation instead of silently serving stale data
  (fetch and push warn on every operation, a later push quarantines the
  damaged generation so the vault recovers instead of locking forever,
  and the commit slot-occupied error distinguishes damage from
  concurrency); a planted
  obj symlink no longer redirects object writes into an arbitrary host
  directory.
- Windows: a crashed holder's vault.lock is now stolen immediately —
  pid liveness is probed via OpenProcess instead of waiting out the
  staleness window (the lock message's delete guidance remains as the
  last resort). Cross-system field testing confirmed every functional
  area on Windows; the remaining field-report items were verified fixed
  in rc.2 (the LFS warning lives in the git-remote-coffer binary and
  fires before the pre-push hook; gitcoffer status warns on fallback).
- Server-side non-fast-forward protection: a vault branch only moves to
  a descendant of its current tip unless the push is forced — unrelated
  or rewritten histories are refused with a remedy instead of silently
  overwriting the branch (the overwritten commits would be unrecoverable
  after gc). The LFS warning now fires before the pre-push hook, and
  `gitcoffer status` warns when the newest manifest generation is
  unreadable and an older one is being served.

- Cross-platform robustness gate passed: the full functional checklist
  ran twice independently on real Windows hardware and on Linux against
  the release-candidate binaries, including cross-system relay on one
  vault and three external-device batteries (exFAT, FAT32, ext4).
- doctor's temp-file scan covers the vault root (interrupted manifest
  writes), matching what gc sweeps — found in cross-platform field
  testing.
- Release trust chain: archives are built with the pinned toolchain
  (CGO disabled, trimpath), checksummed, the checksums signed keylessly
  (Sigstore OIDC via cosign — no stored secrets), an SBOM (spdx-json)
  attached, and GitHub build attestations recorded for every archive.
- CI hardening: race-detector and govulncheck legs, fuzz seeds for the
  untrusted-input parsers (vault.meta, manifest records, chunk framing),
  and a dedicated leg running the full suite against git 2.30 — the
  supported floor is now tested, not asserted.

- `gitcoffer export-bundle <vault> <file>` — the guaranteed exit path: a
  plain, stock-git-readable bundle of everything in the vault.
- `gitcoffer doctor <vault>` — environment and vault health check (git
  version, helper discoverability, meta shape, key-file presence,
  filesystem characteristics, leftover state).
- `gitcoffer gc` — report what would be reclaimed without deleting.
- Pushes from Git-LFS-configured repositories warn that LFS content is
  not part of the vault.
- Independent security review (three cold reviewers) — all confirmed
  findings fixed: unbounded reads of untrusted metadata, crafted
  `vault.meta` shapes, future-dated planted locks, sha256 and shallow
  caller guards, `--atomic` report contract, commit-race narrowing,
  locked meta rewrites, append-tamper detection, vector parameter
  pinning.
- Disk-full robustness suite (user-namespace tmpfs, free-space sweep).
- Security policy ([SECURITY.md](SECURITY.md)); support matrix
  ([docs/support-matrix.md](docs/support-matrix.md)).

## 1.0.0-pre

First pre-release: vault format v1 (frozen, pinned by
[docs/test-vectors.json](docs/test-vectors.json)), remote helper with
progress milestones, writer lock, `--atomic`/`--force-with-lease`,
credential approval, HEAD fallback for non-main repositories; full
`gitcoffer` CLI (init, status, key add/remove/list, rekey, gc, fsck,
version); security-reviewed; six-platform release binaries.
