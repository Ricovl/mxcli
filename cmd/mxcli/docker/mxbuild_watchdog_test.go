// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A build that prints and then goes silent — the shape of Gradle spinning
// behind a wrong --java-home — must be killed by the stall bound, with the
// last output and the java home in the error.
func TestRunWithWatchdog_StallKillsSilentBuild(t *testing.T) {
	// The grandchild `sleep` stands in for the JVM under mxbuild's wrapper: it
	// holds the output pipe, so only a process-tree kill lets Wait return.
	cmd := exec.Command("sh", "-c", "echo starting gradle; echo daemon up; sleep 60 & wait")
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out

	start := time.Now()
	err := runWithWatchdog(cmd, watchdogLimits{Timeout: time.Minute, Stall: 300 * time.Millisecond}, "/usr")
	elapsed := time.Since(start)

	var werr *WatchdogError
	if !errors.As(err, &werr) {
		t.Fatalf("err = %v, want a *WatchdogError", err)
	}
	if werr.Reason != "stalled" {
		t.Errorf("reason = %q, want stalled", werr.Reason)
	}
	if elapsed > 15*time.Second {
		t.Errorf("took %s to give up on a 300ms stall — the process tree was not killed", elapsed)
	}
	msg := err.Error()
	for _, want := range []string{"daemon up", "Java home used: /usr", mxbuildStallEnv} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(out.String(), "starting gradle") {
		t.Errorf("output was not forwarded to the caller's writer: %q", out.String())
	}
}

// A build that keeps printing is not stalled, but the overall bound still
// stops it.
func TestRunWithWatchdog_TimeoutKillsChattyBuild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while true; do echo tick; sleep 0.05; done")
	err := runWithWatchdog(cmd, watchdogLimits{Timeout: 500 * time.Millisecond, Stall: time.Minute}, "/jdk")
	var werr *WatchdogError
	if !errors.As(err, &werr) || werr.Reason != "timed out" {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if !strings.Contains(err.Error(), "tick") || !strings.Contains(err.Error(), mxbuildTimeoutEnv) {
		t.Errorf("error lacks the tail or the env var:\n%s", err)
	}
}

// Control: a build that finishes inside both bounds returns its own result.
func TestRunWithWatchdog_PassesThroughNormalExit(t *testing.T) {
	if err := runWithWatchdog(exec.Command("sh", "-c", "echo ok"), watchdogLimits{Timeout: time.Minute, Stall: time.Minute}, ""); err != nil {
		t.Fatalf("clean exit: %v", err)
	}
	err := runWithWatchdog(exec.Command("sh", "-c", "exit 3"), watchdogLimits{Timeout: time.Minute, Stall: time.Minute}, "")
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("exit 3: err = %v, want the ExitError", err)
	}
}

func TestMxBuildLimits_Env(t *testing.T) {
	t.Setenv(mxbuildTimeoutEnv, "30m")
	t.Setenv(mxbuildStallEnv, "0")
	got := mxbuildLimits()
	if got.Timeout != 30*time.Minute || got.Stall != 0 {
		t.Fatalf("limits = %+v", got)
	}
	t.Setenv(mxbuildTimeoutEnv, "bogus")
	t.Setenv(mxbuildStallEnv, "")
	got = mxbuildLimits()
	if got.Timeout != DefaultMxBuildTimeout || got.Stall != DefaultMxBuildStall {
		t.Fatalf("defaults = %+v", got)
	}
}

// The docker build's MxBuild invocation is bounded: a fake mxbuild that prints
// one line and then hangs (with a grandchild holding the pipe) is killed.
func TestMxBuildCmd_HungBuildIsKilled(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mxbuild")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'Starting Gradle'\nsleep 60 &\nwait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := mxbuildWatchdogLimits
	mxbuildWatchdogLimits = func() watchdogLimits { return watchdogLimits{Timeout: time.Minute, Stall: 300 * time.Millisecond} }
	t.Cleanup(func() { mxbuildWatchdogLimits = orig })

	done := make(chan error, 1)
	var out syncBuffer
	go func() { done <- mxbuildCmd(fake, "/usr", dir, filepath.Join(dir, "App.mpr"), &out, &out) }()
	select {
	case err := <-done:
		var werr *WatchdogError
		if !errors.As(err, &werr) {
			t.Fatalf("err = %v, want a *WatchdogError", err)
		}
		if !strings.Contains(err.Error(), "Starting Gradle") || !strings.Contains(err.Error(), "/usr") {
			t.Errorf("error lacks the tail or the java home:\n%s", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("mxbuildCmd did not return: the hung build was never killed")
	}
}
