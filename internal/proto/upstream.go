package proto

// Upstream auto-attach. A real-world v1.0.0 report: after re-pointing
// origin from a host to a vault, `git pull` silently did nothing and
// VSCode showed "Publish Branch" — the branch had no upstream tracking
// after the switch. After a fetch or push, when the caller's current
// branch tracks nothing anywhere and the coffer remote has a branch of
// the same name, the helper attaches it (main -> origin/main) and says
// what it did plus the undo.
//
// Guards, each with a reason:
//   - non-bare callers only — a bare repo has no current branch;
//   - the local branch ref must exist — a clone mid-fetch has an unborn
//     HEAD, and git's own clone sets tracking anyway;
//   - no upstream anywhere — never steal tracking another remote owns,
//     which would silently repoint a later plain `git pull` in the
//     documented host-plus-vault dual-remote flow;
//   - exact branch-name match on the coffer remote;
//   - pushes attach only when the batch was exactly the current branch —
//     under --all/--mirror, "same name exists" carries no intent;
//   - never under a dry run.
//
// When a guard blocks an otherwise-eligible attach, the hint line prints
// without acting.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ziqing7226/GitCoffer/internal/packproc"
)

// maybeAttachUpstream runs after a completed fetch or push batch.
// pushSpecs is nil after a fetch; after a push it is the batch as git
// sent it. pushFailed suppresses the attach entirely.
func (s *session) maybeAttachUpstream(pushSpecs []string, pushFailed bool) {
	if pushFailed {
		return
	}
	bare, err := packproc.CallerIsBare()
	if err != nil || bare {
		return
	}
	branch, err := packproc.CallerHeadBranch()
	if err != nil || branch == "" {
		return // detached HEAD: no current branch
	}
	if !packproc.CallerBranchExists(branch) {
		return // unborn branch (clone mid-fetch); git sets tracking itself
	}
	if packproc.CallerBranchUpstreamRemote(branch) != "" {
		return // tracks something somewhere: never steal
	}
	if _, ok := s.store.Manifest().Refs["refs/heads/"+branch]; !ok {
		return // the coffer remote has no branch of this name
	}
	remote := s.cofferRemoteName()
	if remote == "" {
		return // cannot name the remote (no configured URL matches): skip
	}

	hint := func() {
		fmt.Fprintf(os.Stderr,
			"git-remote-coffer: hint: branch '%s' has no upstream and the coffer remote has a matching branch — attach it with: git branch --set-upstream-to=%s/%s %s\n",
			branch, remote, branch, branch)
	}
	if s.dryRun {
		hint()
		return
	}
	if pushSpecs != nil {
		// Only a batch that was exactly the current branch carries the
		// intent to attach: --all, --mirror, and explicit multi-branch
		// pushes do not.
		if len(pushSpecs) != 1 || pushSpecDst(pushSpecs[0]) != "refs/heads/"+branch {
			hint()
			return
		}
	}
	if err := packproc.SetBranchUpstream(branch, remote); err != nil {
		fmt.Fprintf(os.Stderr,
			"git-remote-coffer: could not attach '%s' to '%s/%s' (%v) — attach it with: git branch --set-upstream-to=%s/%s %s\n",
			branch, remote, branch, err, remote, branch, branch)
		return
	}
	fmt.Fprintf(os.Stderr,
		"git-remote-coffer: branch '%s' now tracks '%s/%s' — set automatically because the branch had no upstream; undo with: git branch --unset-upstream\n",
		branch, remote, branch)
}

// cofferRemoteName finds the caller's remote name whose effective URL
// addresses this vault ("" when none matches). insteadOf rewrites are
// resolved by git itself via ls-remote --get-url. The comparison folds
// case, matching Windows and default macOS filesystems.
func (s *session) cofferRemoteName() string {
	remotes, err := packproc.CallerRemotes()
	if err != nil {
		return ""
	}
	want := filepath.Clean(s.vaultDir)
	for _, r := range remotes {
		url, err := packproc.CallerRemoteEffectiveURL(r)
		if err != nil || !strings.HasPrefix(url, "coffer::") {
			continue
		}
		got := filepath.Clean(strings.TrimSuffix(strings.TrimPrefix(url, "coffer::"), "/"))
		if strings.EqualFold(got, want) {
			return r
		}
	}
	return ""
}

// pushSpecDst extracts the destination ref of one push specification
// ("<src>:<dst>", optional leading +). Malformed specs yield "".
func pushSpecDst(spec string) string {
	spec = strings.TrimPrefix(spec, "+")
	i := strings.Index(spec, ":")
	if i < 0 {
		return ""
	}
	return spec[i+1:]
}
