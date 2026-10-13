# Contributing to Coffer

Thanks for helping. Coffer is a security tool: clarity beats cleverness, and
the specifications are contracts.

## Iron Rule — English only

Every artifact produced in this repository — code, comments, documentation,
commit messages, issues, pull request descriptions, and release notes — must
be written entirely in English. No exceptions, no mixed language. Reviewers
and release checks treat this as a hard requirement.

## Before you invest time

- **Now:** the best contributions are issues that find holes, ambiguities,
  or over-engineering in the documentation and implementation, and pull
  requests that fix them.
- **Implementation work:** code follows docs/development.md. Check that the
  relevant phase is active before proposing implementation, and open an
  issue first for anything not already covered by one.

## Commit style

Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`), one
logical change per commit, imperative subject line, body explaining the
*why*.

## Versioning

Three positive integers, `a.b.c`, tagged as `v<a.b.c>`:

- **a — major direction** (currently 1). Moves only for an
  architectural or direction-level change.
- **b — minor direction.** Work on a new minor direction begins only
  after its goals are written into docs/development.md (see the work
  plan there); b itself moves up when that direction's first stable
  lands.
- **c — release-candidate counter of the current minor direction.**
  c increments and tags only when local self-checks pass, CI is fully
  green, and the build is ready for the broader, deeper robustness
  test round. When a candidate passes everything, the stable ships as
  `a.(b+1).0`: c returns to 0 and b moves up.

**Release channels follow the counter: every `c > 0` tag is published
as a pre-release; every `c = 0` tag is a full stable release.**

Consequences:

- Any `c = 0` version is stable; a higher b is always newer within the
  same a; a stays 1 unless the architecture changes.
- `1.0.23` is a legal version: twenty-three rc attempts toward 1.1.0,
  none stable yet — the counter carries the honest history.
- The first stable under this scheme is **1.1.0**, whose goals are
  already recorded in the development work plan.

## Pull request checklist

- [ ] English only (see above).
- [ ] Tests cover the change, and golden protocol tests pass on Linux,
      Windows, and macOS (once the code phases are active).
- [ ] Behavior-affecting change: docs/format-spec.md or docs/architecture.md
      updated in the same PR, with version fields bumped where applicable.
- [ ] No real names, personal paths, or machine-specific details anywhere.
