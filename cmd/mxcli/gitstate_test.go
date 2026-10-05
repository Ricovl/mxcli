// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#972 item 3: warn about git states that crash Studio Pro 11.13.

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("CI", "")
	t.Setenv(noGitWarningsEnv, "")
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepoWithProject makes a repository holding App.mpr on branch "feature",
// with one commit, and returns the .mpr path.
func newRepoWithProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Keep git from discovering a repository above the temp dir.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	gitIn(t, dir, "init", "-q", "-b", "feature")
	mprPath := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mprPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "App.mpr")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	return mprPath
}

func TestGitState_NoUpstreamWarns(t *testing.T) {
	requireGit(t)
	mprPath := newRepoWithProject(t)
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare")
	gitIn(t, filepath.Dir(mprPath), "remote", "add", "origin", remote)

	got := gitStateWarnings(mprPath)
	if len(got) != 1 || !strings.Contains(got[0], `branch "feature" has no upstream`) ||
		!strings.Contains(got[0], "git push -u origin feature") {
		t.Fatalf("want a no-upstream warning with the push remedy, got %q", got)
	}
}

func TestGitState_NoRemoteSaysAddOne(t *testing.T) {
	requireGit(t)
	mprPath := newRepoWithProject(t)
	got := gitStateWarnings(mprPath)
	if len(got) != 1 || !strings.Contains(got[0], "git remote add origin <url> && git push -u origin feature") {
		t.Fatalf("want a no-upstream warning with the add-remote remedy, got %q", got)
	}
}

func TestGitState_WithUpstreamIsSilent(t *testing.T) {
	requireGit(t)
	mprPath := newRepoWithProject(t)
	remote := t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare")
	dir := filepath.Dir(mprPath)
	gitIn(t, dir, "remote", "add", "origin", remote)
	gitIn(t, dir, "push", "-q", "-u", "origin", "feature")

	if got := gitStateWarnings(mprPath); len(got) != 0 {
		t.Fatalf("control: branch with upstream warned: %q", got)
	}
}

func TestGitState_NotARepoIsSilent(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	if got := gitStateWarnings(filepath.Join(dir, "App.mpr")); len(got) != 0 {
		t.Fatalf("outside a repository warned: %q", got)
	}
}

func TestGitState_DetachedHeadIsSilent(t *testing.T) {
	requireGit(t)
	mprPath := newRepoWithProject(t)
	gitIn(t, filepath.Dir(mprPath), "checkout", "-q", "--detach")
	if got := gitStateWarnings(mprPath); len(got) != 0 {
		t.Fatalf("detached HEAD warned: %q", got)
	}
}

func TestGitState_CIAndOptOutAreSilent(t *testing.T) {
	requireGit(t)
	mprPath := newRepoWithProject(t)
	t.Setenv("CI", "true")
	if got := gitStateWarnings(mprPath); len(got) != 0 {
		t.Fatalf("CI warned: %q", got)
	}
	t.Setenv("CI", "")
	t.Setenv(noGitWarningsEnv, "1")
	if got := gitStateWarnings(mprPath); len(got) != 0 {
		t.Fatalf("opt-out warned: %q", got)
	}
}

// Dubious ownership needs a repository owned by another uid, which a test
// cannot create; git's refusal is replayed through the runner instead. The
// stderr is git 2.43's, verbatim.
func TestGitState_DubiousOwnershipWarnsWithGitsRemedy(t *testing.T) {
	const stderr = "fatal: detected dubious ownership in repository at '/workspaces/app'\n" +
		"To add an exception for this directory, call:\n\n" +
		"\tgit config --global --add safe.directory /workspaces/app\n"
	git := func(dir string, args ...string) (string, string, error) {
		return "", stderr, errors.New("exit status 128")
	}
	got := gitStateWarningsWith("/workspaces/app/sub", git)
	if len(got) != 1 || !strings.Contains(got[0], "dubious ownership") ||
		!strings.Contains(got[0], "git config --global --add safe.directory /workspaces/app") {
		t.Fatalf("want a dubious-ownership warning with git's remedy, got %q", got)
	}
}
