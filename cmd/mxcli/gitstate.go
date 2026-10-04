// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitstate.go warns about git states that crash Studio Pro (ako/mxcli#972).
//
// Studio Pro 11.13 (reported on macOS) fails to open a project with "Unable to
// find 'system' property in 'system'" when the project folder is a git
// repository whose checked-out branch has no upstream, or which git refuses
// with "detected dubious ownership" (a folder shared between the host and a
// devcontainer, owned by a different uid on each side). Both are Studio Pro
// bugs; mxcli cannot fix them, but it can say so before the user opens the
// project and has to work out what the crash means.
//
// Policy — the warnings are advice, so they are quiet whenever they could be
// wrong or unwanted:
//   - never fail the command; any error running git yields no warning;
//   - nothing outside a git repository, and nothing when git is not installed;
//   - nothing on a detached HEAD: that is how CI and `git checkout <sha>` leave
//     a repository, there is no branch to push, and nobody opens such a
//     checkout in Studio Pro;
//   - nothing when $CI is set, or when MXCLI_NO_GIT_WARNINGS=1.
//
// Dubious ownership is judged by the git this process runs. In a devcontainer
// that is the container's git, and the host's Studio Pro may see ownership
// differently — so the check catches the case when the container sees it, and
// the docs give the host-side remedy either way.

// noGitWarningsEnv silences the warnings.
const noGitWarningsEnv = "MXCLI_NO_GIT_WARNINGS"

// gitRunner runs git in dir and returns stdout, stderr and the error.
type gitRunner func(dir string, args ...string) (string, string, error)

// runGitState is the default gitRunner, bounded so a hung git cannot stall a check.
func runGitState(dir string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	// Never prompt (credential helpers, pagers) from a warning.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat")
	err := cmd.Run()
	return strings.TrimSpace(out.String()), errb.String(), err
}

// projectDirOf returns the folder of a project path (.mpr file or folder).
func projectDirOf(projectPath string) string {
	if fi, err := os.Stat(projectPath); err == nil && fi.IsDir() {
		return projectPath
	}
	return filepath.Dir(projectPath)
}

// gitStateWarnings returns the warnings for projectPath's git state, if any.
func gitStateWarnings(projectPath string) []string {
	if projectPath == "" || os.Getenv("CI") != "" || os.Getenv(noGitWarningsEnv) == "1" {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	return gitStateWarningsWith(projectDirOf(projectPath), runGitState)
}

func gitStateWarningsWith(dir string, git gitRunner) []string {
	out, stderr, err := git(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if strings.Contains(stderr, "dubious ownership") {
			return []string{dubiousOwnershipWarning(dir, stderr)}
		}
		return nil // not a repository (or git failed): say nothing
	}
	if out != "true" {
		return nil // inside .git, or a bare repository
	}
	branch, _, err := git(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch == "" {
		return nil // detached HEAD: see the policy above
	}
	if _, _, err := git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		return nil
	}
	remotes, _, _ := git(dir, "remote")
	remote := "origin"
	if rs := strings.Fields(remotes); len(rs) > 0 && !slices.Contains(rs, "origin") {
		remote = rs[0]
	}
	remedy := fmt.Sprintf("git push -u %s %s", remote, branch)
	if strings.TrimSpace(remotes) == "" {
		remedy = fmt.Sprintf("git remote add origin <url> && git push -u origin %s", branch)
	}
	return []string{fmt.Sprintf(
		"Warning: git branch %q has no upstream. Studio Pro 11.13 fails to open a project in this state "+
			"(\"Unable to find 'system' property in 'system'\"). Before opening it in Studio Pro, run: %s",
		branch, remedy)}
}

// dubiousOwnershipWarning reuses the safe.directory command git itself
// suggests (it names the repository root, which may be above dir).
func dubiousOwnershipWarning(dir, stderr string) string {
	remedy := ""
	for _, line := range strings.Split(stderr, "\n") {
		if i := strings.Index(line, "git config --global --add safe.directory"); i >= 0 {
			remedy = strings.TrimSpace(line[i:])
			break
		}
	}
	if remedy == "" {
		remedy = fmt.Sprintf("git config --global --add safe.directory %s", dir)
	}
	return "Warning: git reports \"detected dubious ownership\" for this project (typically a folder shared " +
		"with a devcontainer). Studio Pro 11.13 fails to open a project in this state " +
		"(\"Unable to find 'system' property in 'system'\"). On the machine that runs Studio Pro, run: " + remedy
}

// printGitStateWarnings writes each warning on its own line to w.
func printGitStateWarnings(projectPath string, w io.Writer) {
	for _, msg := range gitStateWarnings(projectPath) {
		fmt.Fprintln(w, msg)
	}
}
