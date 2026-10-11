# Packaging manifests

Submission templates for the package stores. They target the stable
1.0.0 tag; the `sha256`/`hash` fields are filled from that release's
`checksums.txt` at submission time (the current placeholders are
intentionally invalid — never ship a manifest with an unverified
hash).

| Store | Manifest | Destination |
|---|---|---|
| Homebrew | [homebrew/gitcoffer.rb](homebrew/gitcoffer.rb) | homebrew-core (or a custom tap) |
| scoop | [scoop/gitcoffer.json](scoop/gitcoffer.json) | ScoopInstaller/Extras or a personal bucket |
| winget | [winget/Ziqing7226.GitCoffer.yaml](winget/Ziqing7226.GitCoffer.yaml) | microsoft/winget-pkgs |

Until then, users install from the [release archives](https://github.com/Ziqing7226/GitCoffer/releases),
with `go install github.com/Ziqing7226/GitCoffer/cmd/{gitcoffer,git-remote-coffer}@<tag>`, or with the
one-step installers (`scripts/install.sh`, `scripts/install.ps1`).

`docs/assets/logo-badge.png` is the repository's social-avatar image
(set it under GitHub → Settings → General); it is deliberately not
embedded in any page.
