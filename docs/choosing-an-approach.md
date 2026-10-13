# Choosing an approach

Which copy actually needs encrypting? Usually not the server. Servers —
including self-hosted forges like Gitea and GitLab, which keep
repositories in plaintext by design and rely on full-disk encryption on
the machine in the rack — are not the weak link. The weak link is the
copy that leaves: the portable drive carried offsite, in bags, between
machines, that eventually gets lost. Once it leaves the server, it is
outside every one of those protections.

There are four mature shapes for encrypting that copy, each fitting
different needs:

- **Encrypting individual files inside the repository** — the natural
  fit when the remote host must stay readable, for example while
  collaborating through a public git host.
- **An encrypted volume holding a bare repository** — natural when you
  already work with encrypted volumes and don't mind mounting one
  before every push.
- **A general backup tool archiving the repository** (restic, Borg,
  Kopia) — natural when one tool already backs up everything else on
  the machine. Its artifact is an archive restored with that same
  tool; Coffer's artifact is a remote you clone like any other.
- **An encrypted remote helper** — the whole remote is ciphertext;
  `git push` stays a single step, on every platform, with no drivers or
  daemons. For the offsite copy, this turns the backup job itself into
  `git push` — no plaintext copy is ever written. Two honest
  boundaries: a vault stores git data (commits, branches, tags), not a
  forge's issues, wikis, or CI records — keep the platform's own dump
  for those.

Coffer lives in the fourth shape. Its design follows the path that
[git-remote-gcrypt](https://github.com/spwhitton/git-remote-gcrypt)
proved on Linux, and aims to offer it everywhere with no dependencies —
we are grateful for the ground it broke.
