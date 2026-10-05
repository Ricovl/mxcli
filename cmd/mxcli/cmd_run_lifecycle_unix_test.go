// SPDX-License-Identifier: Apache-2.0

//go:build linux

package main

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// fakeDetachedRun starts a stand-in for a detached `run --local`: a session
// leader that ignores nothing and has a child of its own that outlives it — the
// shape of mxcli + its runtime JVM. It returns the leader's pid once the child
// is up.
func fakeDetachedRun(t *testing.T) (*exec.Cmd, time.Time) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 60 & echo up; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, _ := cmd.StdoutPipe()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sh: %v", err)
	}
	buf := make([]byte, 8)
	_, _ = io.ReadAtLeast(out, buf, 2)
	go func() { _ = cmd.Wait() }() // reap the leader when it dies
	t.Cleanup(func() {
		for _, pid := range docker.SessionMembers(cmd.Process.Pid, time.Time{}) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return cmd, start
}

// SIGTERM to the leader ends it, but its child — like a runtimelauncher whose
// mxcli died before its teardown ran — keeps running in the session. run stop
// must find and stop it, and must not report success while it lives.
func TestRunStop_LeavesNoOrphanBehind(t *testing.T) {
	fastPolling(t)
	mpr := scratchProject(t)
	leader, start := fakeDetachedRun(t)
	pid := leader.Process.Pid
	if n := len(docker.SessionMembers(pid, start)); n < 2 {
		t.Fatalf("fake run has %d session members, want the leader and its child", n)
	}
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: pid, Detached: true, Phase: docker.PhaseRunning, Started: start})

	var out bytes.Buffer
	code := runStop(mpr, &out)
	if code != runExitOK {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if left := docker.SessionMembers(pid, start); len(left) != 0 {
		t.Fatalf("orphans left after run stop: %v (output %q)", left, out.String())
	}
	if !strings.HasPrefix(out.String(), "stopped: pid ") || !strings.Contains(out.String(), "reaped 1 leftover") {
		t.Errorf("output %q", out.String())
	}
	st, _ := docker.ReadRunState(mpr)
	if st == nil || st.Phase != docker.PhaseStopped {
		t.Errorf("state after stop: %+v", st)
	}
}

// A run killed with -9 ran no teardown: the leader is gone, its children are
// not. status must say so, and stop must clean them up.
func TestRunStop_ReapsTheLeftoversOfAKilledRun(t *testing.T) {
	fastPolling(t)
	mpr := scratchProject(t)
	leader, start := fakeDetachedRun(t)
	pid := leader.Process.Pid
	_ = docker.WriteRunState(mpr, &docker.RunState{PID: pid, Detached: true, Phase: docker.PhaseRunning, Started: start, URL: "u"})
	_ = syscall.Kill(pid, syscall.SIGKILL)
	waitGone(func() bool { return docker.PidAlive(pid) }, 5*time.Second)

	var out bytes.Buffer
	if code := runStatus(mpr, &out); code != runExitNotRunning || !strings.Contains(out.String(), "1 leftover process(es)") {
		t.Errorf("status: exit %d, %q", code, out.String())
	}
	out.Reset()
	if code := runStop(mpr, &out); code != runExitOK || out.String() != "stopped (was not running, reaped 1 leftover process(es))\n" {
		t.Errorf("stop: exit %d, %q", code, out.String())
	}
	if left := docker.SessionMembers(pid, start); len(left) != 0 {
		t.Errorf("orphans left: %v", left)
	}
}
