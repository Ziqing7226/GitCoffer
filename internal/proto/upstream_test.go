package proto

// Session-level tests for upstream auto-attach (see upstream.go for the
// field report that motivated it). The e2e suite drives the same
// behavior through real git; these pin the guards at the conversation
// level, where hand-crafted batches reach shapes git rarely sends.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ziqing7226/GitCoffer/internal/crypto"
	"github.com/Ziqing7226/GitCoffer/internal/vault"
)

// driveSessionErr is driveSession with stderr captured too.
func driveSessionErr(t *testing.T, repoDir, vaultDir string, lines ...string) (stdout, stderr string) {
	t.Helper()
	askpass := filepath.Join(t.TempDir(), "askpass")
	if err := os.WriteFile(askpass, []byte("#!/bin/sh\necho test-pass\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wdir, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(wdir)

	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errOut strings.Builder
	oldAskpass := os.Getenv("GIT_ASKPASS")
	os.Setenv("GIT_ASKPASS", askpass)
	defer os.Setenv("GIT_ASKPASS", oldAskpass)

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	runErr := Run(vaultDir, in, &out)
	os.Stderr = oldStderr
	w.Close()
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		errOut.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if runErr != nil {
		t.Fatalf("session aborted: %v", runErr)
	}
	return out.String(), errOut.String()
}

// callerRepo builds a repository with one commit on main and origin
// pointed at the vault — the migrated-repository shape from the report.
func callerRepo(t *testing.T, vaultDir string) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", "."},
		{"config", "user.name", "T"},
		{"config", "user.email", "t@inv.alid"},
		{"remote", "add", "origin", "coffer::" + vaultDir},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644)
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "c"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func upstreamRemote(t *testing.T, repo, branch string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", "branch."+branch+".remote")
	cmd.Dir = repo
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func mustVault(t *testing.T) string {
	t.Helper()
	vaultDir := t.TempDir()
	if _, err := vault.Create(vaultDir, "test-pass", crypto.Argon2Params{Algo: crypto.KDFAlgo, M: 16, T: 1, P: 1}); err != nil {
		t.Fatal(err)
	}
	return vaultDir
}

func TestUpstreamAttachAfterPushSession(t *testing.T) {
	vaultDir := mustVault(t)
	repo := callerRepo(t, vaultDir)

	_, stderr := driveSessionErr(t, repo, vaultDir,
		"list for-push",
		"push HEAD:refs/heads/main",
		"",
	)
	if !strings.Contains(stderr, "now tracks 'origin/main'") {
		t.Fatalf("push session lacks attach message:\n%s", stderr)
	}
	if got := upstreamRemote(t, repo, "main"); got != "origin" {
		t.Fatalf("branch.main.remote = %q, want origin", got)
	}
	if !strings.Contains(stderr, "--unset-upstream") {
		t.Fatalf("attach message lacks the undo:\n%s", stderr)
	}
}

func TestUpstreamAttachAfterFetchSession(t *testing.T) {
	vaultDir := mustVault(t)
	repo := callerRepo(t, vaultDir)

	driveSessionErr(t, repo, vaultDir,
		"list for-push",
		"push HEAD:refs/heads/main",
		"",
	)
	// The migrated shape: a second repository with its own history
	// fetches from the vault — the "pull did nothing" scenario.
	mig := callerRepo(t, vaultDir)
	oidCmd := exec.Command("git", "rev-parse", "HEAD")
	oidCmd.Dir = repo
	oidOut, err := oidCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimSpace(string(oidOut))

	_, stderr := driveSessionErr(t, mig, vaultDir,
		"list",
		"fetch "+oid,
		"",
	)
	if !strings.Contains(stderr, "now tracks 'origin/main'") {
		t.Fatalf("fetch session lacks attach message:\n%s", stderr)
	}
	if got := upstreamRemote(t, mig, "main"); got != "origin" {
		t.Fatalf("branch.main.remote = %q, want origin", got)
	}
}

func TestUpstreamNeverStealsExistingTracking(t *testing.T) {
	vaultDir := mustVault(t)
	repo := callerRepo(t, vaultDir)
	// Tracking configured against another remote (the dual-remote flow):
	// the attach must not touch it.
	for _, kv := range [][2]string{
		{"branch.main.remote", "github"},
		{"branch.main.merge", "refs/heads/main"},
	} {
		cmd := exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v\n%s", kv[0], err, out)
		}
	}

	_, stderr := driveSessionErr(t, repo, vaultDir,
		"list for-push",
		"push HEAD:refs/heads/main",
		"",
	)
	if strings.Contains(stderr, "now tracks") {
		t.Fatalf("attach stole existing tracking:\n%s", stderr)
	}
	if got := upstreamRemote(t, repo, "main"); got != "github" {
		t.Fatalf("branch.main.remote = %q, want github (untouched)", got)
	}
}

func TestUpstreamGuardsHintWithoutActing(t *testing.T) {
	vaultDir := mustVault(t)
	repo := callerRepo(t, vaultDir)

	// The vault first needs the branch, so the dry-run precondition
	// (coffer remote has a same-named branch) can hold.
	driveSessionErr(t, repo, vaultDir,
		"list for-push",
		"push HEAD:refs/heads/main",
		"",
	)
	gitCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("branch", "--unset-upstream")

	// A multi-ref batch (--all/--mirror carry no intent): hint, no attach.
	_, stderr := driveSessionErr(t, repo, vaultDir,
		"list for-push",
		"push HEAD:refs/heads/main",
		"push HEAD:refs/heads/other",
		"",
	)
	if !strings.Contains(stderr, "hint:") || !strings.Contains(stderr, "--set-upstream-to=") {
		t.Fatalf("multi-ref push lacks the hint:\n%s", stderr)
	}
	if strings.Contains(stderr, "now tracks") {
		t.Fatalf("multi-ref push attached anyway:\n%s", stderr)
	}
	if got := upstreamRemote(t, repo, "main"); got != "" {
		t.Fatalf("multi-ref push attached: branch.main.remote = %q", got)
	}

	// A dry run: hint, no attach.
	_, stderr = driveSessionErr(t, repo, vaultDir,
		"option dry-run true",
		"list for-push",
		"push HEAD:refs/heads/main",
		"",
	)
	if !strings.Contains(stderr, "hint:") {
		t.Fatalf("dry-run push lacks the hint:\n%s", stderr)
	}
	if got := upstreamRemote(t, repo, "main"); got != "" {
		t.Fatalf("dry-run push attached: branch.main.remote = %q", got)
	}
}

func TestPushSpecDst(t *testing.T) {
	for spec, want := range map[string]string{
		"HEAD:refs/heads/main":  "refs/heads/main",
		"+HEAD:refs/heads/main": "refs/heads/main",
		":refs/heads/gone":      "refs/heads/gone",
		"refs/heads/main":       "",
	} {
		if got := pushSpecDst(spec); got != want {
			t.Errorf("pushSpecDst(%q) = %q, want %q", spec, got, want)
		}
	}
}
