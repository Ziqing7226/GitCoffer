package e2e

// Upstream auto-attach end to end, from the v1.0.0 field report: after
// re-pointing origin from a host to a vault, a plain `git pull` silently
// did nothing and VSCode showed "Publish Branch" — the branch had no
// upstream tracking after the switch. Real git drives the whole flow;
// the guards are pinned separately at the session level.

import (
	"path/filepath"
	"strings"
	"testing"
)

func upstreamRemoteOf(t *testing.T, repo, branch string) string {
	t.Helper()
	out, err := runGit(repo, nil, "config", "--get", "branch."+branch+".remote")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func TestUpstreamAutoAttachEndToEnd(t *testing.T) {
	vaultDir := newVault(t)
	src := newRepo(t, "src")
	git(t, src, nil, "remote", "add", "origin", vaultURL(vaultDir))

	// A fetch against an empty vault must not attach: no matching branch
	// exists yet, so there is nothing to say.
	git(t, src, nil, "fetch", "-q", "origin")
	if got := upstreamRemoteOf(t, src, "main"); got != "" {
		t.Fatalf("attach fired before the vault had a matching branch: %q", got)
	}

	// A plain push (no -u) attaches main to origin/main and says so,
	// with the undo.
	pushOut := git(t, src, nil, "push", "origin", "main")
	if !strings.Contains(pushOut, "now tracks 'origin/main'") {
		t.Fatalf("push output lacks the attach line: %s", pushOut)
	}
	if got := upstreamRemoteOf(t, src, "main"); got != "origin" {
		t.Fatalf("branch.main.remote = %q, want origin", got)
	}
	if got := strings.TrimSpace(git(t, src, nil, "rev-parse", "--abbrev-ref", "main@{u}")); got != "origin/main" {
		t.Fatalf("main@{u} = %q, want origin/main", got)
	}

	// The printed undo actually undoes.
	git(t, src, nil, "branch", "--unset-upstream")
	if got := upstreamRemoteOf(t, src, "main"); got != "" {
		t.Fatalf("unset-upstream left tracking behind: %q", got)
	}

	// The fetch path — the second-machine scenario: another clone pushes
	// new work, and a repository without upstream tracking fetches it.
	// (A fetch with nothing to transfer sends the helper no batch, so the
	// attach rides the fetch that does real work — the same condition
	// under which a pull would have moved the branch.)
	clone1 := filepath.Join(t.TempDir(), "clone1")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(vaultDir), clone1)
	git(t, clone1, nil, "config", "user.name", testGitName)
	git(t, clone1, nil, "config", "user.email", testGitMail)
	commit(t, clone1, "file.txt", "one\ntwo\n", "commit two")
	git(t, clone1, nil, "push", "-q", "origin", "main")

	git(t, src, nil, "fetch", "-q", "origin")
	if got := upstreamRemoteOf(t, src, "main"); got != "origin" {
		t.Fatalf("fetch did not re-attach: %q", got)
	}
	git(t, src, nil, "branch", "--unset-upstream")
	// Catch src up so the later push is a fast-forward.
	git(t, src, nil, "merge", "-q", "--ff-only", "origin/main")

	// Never steal: tracking configured against another remote (the
	// documented host-plus-vault dual-remote flow) stays untouched.
	git(t, src, nil, "remote", "add", "host", "https://example.invalid/src.git")
	git(t, src, nil, "config", "branch.main.remote", "host")
	git(t, src, nil, "config", "branch.main.merge", "refs/heads/main")
	out := git(t, src, nil, "push", "origin", "main")
	if strings.Contains(out, "now tracks") {
		t.Fatalf("attach stole tracking from another remote: %s", out)
	}
	if got := upstreamRemoteOf(t, src, "main"); got != "host" {
		t.Fatalf("branch.main.remote = %q, want host (untouched)", got)
	}

	// Multi-branch pushes do not attach (the --all/--mirror guard).
	vault2 := newVault(t)
	multi := newRepo(t, "multi")
	git(t, multi, nil, "branch", "feature")
	git(t, multi, nil, "remote", "add", "origin", vaultURL(vault2))
	out = git(t, multi, nil, "push", "origin", "main", "feature")
	if strings.Contains(out, "now tracks") {
		t.Fatalf("multi-branch push attached: %s", out)
	}
	if !strings.Contains(out, "hint:") {
		t.Fatalf("multi-branch push lacks the hint: %s", out)
	}
	if got := upstreamRemoteOf(t, multi, "main"); got != "" {
		t.Fatalf("multi-branch push attached: %q", got)
	}

	// Dry runs never attach (and say what to run instead). The dry-run
	// needs a ref update to send a batch at all.
	commit(t, multi, "file.txt", "one\ndry\n", "commit dry")
	out = git(t, multi, nil, "push", "--dry-run", "origin", "main")
	if strings.Contains(out, "now tracks") {
		t.Fatalf("dry-run push attached: %s", out)
	}
	if !strings.Contains(out, "hint:") {
		t.Fatalf("dry-run push lacks the hint: %s", out)
	}
	if got := upstreamRemoteOf(t, multi, "main"); got != "" {
		t.Fatalf("dry-run push attached: %q", got)
	}

	// A clone attaches nothing itself — git's own clone setup owns that
	// — and the vault-remote relationship it configures matches what the
	// helper would have attached.
	clone := filepath.Join(t.TempDir(), "clone")
	git(t, t.TempDir(), nil, "clone", "-q", vaultURL(vaultDir), clone)
	if got := upstreamRemoteOf(t, clone, "main"); got != "origin" {
		t.Fatalf("clone's own tracking missing: %q", got)
	}
}
