package packproc

// Tests for the caller-repository upstream attachment writes. These run
// git for real in a scratch repository (the config form is what a
// helper conversation can rely on — the remote-tracking ref does not
// exist yet — so that is the path under test).

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", "."},
		{"config", "user.name", "T"},
		{"config", "user.email", "t@inv.alid"},
		{"remote", "add", "origin", "coffer::/tmp/whatever"},
		{"commit", "--allow-empty", "-qm", "c"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func inRepo(t *testing.T, dir string, f func()) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	f()
}

func configGet(t *testing.T, dir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func TestSetBranchUpstreamConfigForm(t *testing.T) {
	dir := initRepo(t)
	// No remote-tracking ref exists, so the plain --set-upstream-to
	// fails and the config form takes over — the shape a helper
	// conversation always sees.
	inRepo(t, dir, func() {
		if err := SetBranchUpstream("main", "origin"); err != nil {
			t.Fatalf("SetBranchUpstream: %v", err)
		}
	})
	if got := configGet(t, dir, "branch.main.remote"); got != "origin" {
		t.Errorf("branch.main.remote = %q", got)
	}
	if got := configGet(t, dir, "branch.main.merge"); got != "refs/heads/main" {
		t.Errorf("branch.main.merge = %q", got)
	}
}

func TestSetBranchUpstreamDashBranchUsesConfigForm(t *testing.T) {
	dir := initRepo(t)
	// A dash-prefixed branch is not valid to `git branch` on a command
	// line (option parsing); the config form must carry it.
	cmd := exec.Command("git", "update-ref", "refs/heads/-x", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-ref: %v\n%s", err, out)
	}
	inRepo(t, dir, func() {
		if err := SetBranchUpstream("-x", "origin"); err != nil {
			t.Fatalf("SetBranchUpstream(-x): %v", err)
		}
	})
	if got := configGet(t, dir, "branch.-x.remote"); got != "origin" {
		t.Errorf("branch.-x.remote = %q", got)
	}
	if got := configGet(t, dir, "branch.-x.merge"); got != "refs/heads/-x" {
		t.Errorf("branch.-x.merge = %q", got)
	}
}

func TestSetBranchUpstreamRefusesToSteal(t *testing.T) {
	dir := initRepo(t)
	if out, err := exec.Command("git", "-C", dir, "config", "branch.main.remote", "github").CombinedOutput(); err != nil {
		t.Fatalf("seed config: %v\n%s", err, out)
	}
	inRepo(t, dir, func() {
		err := SetBranchUpstream("main", "origin")
		if err == nil || !strings.Contains(err.Error(), "gained an upstream") {
			t.Fatalf("SetBranchUpstream over existing tracking = %v, want refusal", err)
		}
	})
	if got := configGet(t, dir, "branch.main.remote"); got != "github" {
		t.Errorf("existing tracking clobbered: %q", got)
	}
}
