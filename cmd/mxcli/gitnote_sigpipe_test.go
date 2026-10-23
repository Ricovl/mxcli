// SPDX-License-Identifier: Apache-2.0

//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1490: `git note --write | head -1` wrote no note and said
// nothing. A Go program that writes to a stdout whose reader has gone is ended by
// SIGPIPE, and the command printed each note before writing it, so the first
// print was where the write the user asked for stopped. The pipeline reported
// head's status, 0.
//
// The reader here is closed before mxcli starts: `| head -1` with the race taken
// out, since head only loses it when a note is longer than the pipe buffer. Two
// commits, because writing each note just before its own print still loses
// every note after the first.
func TestGitNote_WriteSurvivesAClosedStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a git repository")
	}
	requireGit(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	gitIn(t, dir, "init", "-q", "-b", "feature")
	mprPath := filepath.Join(dir, "App.mpr")

	units := baseUnits(t)
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	for _, title := range []string{"Welcome", "Hello"} {
		units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: title})}
		writeProject(t, dir, "11.13.0", units)
		gitIn(t, dir, "commit", "-q", "-am", "retitle home: "+title)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()

	enc, _ := json.Marshal([]string{"git", "note", "-p", mprPath, "HEAD~2..HEAD", "--write"})
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunMainHelper$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), runMainEnv+"="+string(enc), "MXCLI_LOG_DIR="+t.TempDir(), "MXCLI_QUIET=1")
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = w, &stderr
	runErr := cmd.Run()

	// The control: the pipe has to break, or this proves nothing about #1490.
	// It still does after the fix, on the first print — after the writes.
	var ee *exec.ExitError
	if !errors.As(runErr, &ee) {
		t.Fatalf("mxcli must hit the closed stdout (exit 141, as reported); got %v\nstderr:\n%s", runErr, stderr.String())
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Fatalf("mxcli must end on SIGPIPE, as reported; got %v\nstderr:\n%s", runErr, stderr.String())
	}

	for _, rev := range []string{"HEAD~1", "HEAD"} {
		if note := gitNoteOrEmpty(dir, rev); note == "" {
			t.Errorf("%s: no mx_metadata note — `git note --write` lost it to a closed stdout", rev)
		}
	}
}

// gitNoteOrEmpty is rev's mx_metadata note, or "" when it has none.
func gitNoteOrEmpty(dir, rev string) string {
	out, err := exec.Command("git", "-C", dir, "notes", "--ref="+mxMetadataRef, "show", rev).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
