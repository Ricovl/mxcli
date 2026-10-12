// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// #1396: "mxcli test --local hangs indefinitely ... after the tests finish, when
// mxbuild starts a new Gradle daemon". The daemon inherits the write end of the
// output pipe exec.Cmd creates for a non-*os.File writer, outlives mxbuild, and
// escapes the tree kill — so Wait, waiting for EOF, never returns.
//
// The fakes reproduce exactly that shape: a grandchild started with setsid
// (out of mxbuild's process group, as the daemon is out of taskkill /T's reach
// once its parent is gone) holds stdout open long after the direct child exits.

// orphanFake writes an executable script that starts a setsid'd grandchild
// holding stdout for 30s, records its PID, and then runs body. The grandchild
// is killed when the test ends.
func orphanFake(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "daemon.pid")
	script := filepath.Join(dir, "fake-mx")
	src := "#!/bin/sh\n" +
		"echo starting gradle daemon\n" +
		"setsid sh -c 'echo $$ > \"" + pidFile + "\"; exec sleep 30' &\n" +
		// Wait until the daemon has recorded itself, so cleanup can reap it.
		"while [ ! -s \"" + pidFile + "\" ]; do sleep 0.01; done\n" +
		body + "\n"
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return script
}

func shortOrphanWait(t *testing.T) {
	t.Helper()
	old := orphanPipeWaitDelay
	orphanPipeWaitDelay = 300 * time.Millisecond
	t.Cleanup(func() { orphanPipeWaitDelay = old })
}

// ServeServer.Stop is what `test --local` runs before it prints results and
// writes the JUnit file; it must return once mxbuild itself has gone.
func TestServeStop_ReturnsWhenOrphanedDaemonHoldsPipe(t *testing.T) {
	shortOrphanWait(t)
	// The serve process blocks until signalled, like `mxbuild --serve`.
	fake := orphanFake(t, "exec sleep 60")

	cmd, log := newServeCmd(fake, "127.0.0.1", 6543, "/jdk", "/jdk/bin/java")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &ServeServer{Host: "127.0.0.1", Port: 6543, cmd: cmd, log: log}
	// Let the fake reach its blocking exec before it is signalled.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(log.String(), "starting gradle daemon") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	done := make(chan struct{})
	start := time.Now()
	go func() { _ = s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("Stop() still blocked after 15s: an orphaned daemon holding mxbuild's output pipe hangs teardown (#1396)")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Stop() took %s", elapsed)
	}
}

// `mx sync-java-dependencies` runs Gradle too. Its own exit is success even if
// the daemon it started is still holding the pipe.
func TestSyncJavaDependencies_ReturnsWhenOrphanedDaemonHoldsPipe(t *testing.T) {
	shortOrphanWait(t)
	fake := orphanFake(t, "echo synced 1 jar; exit 0")
	project := filepath.Join(t.TempDir(), "App.mpr")

	type result struct{ err error }
	done := make(chan result, 1)
	start := time.Now()
	var out strings.Builder
	go func() { done <- result{SyncJavaDependencies(project, fake, "11.12.5", &out)} }()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("mx exited 0 but SyncJavaDependencies failed: %v", r.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("SyncJavaDependencies still blocked after 15s: an orphaned daemon holding mx's output pipe hangs it (#1396)")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("SyncJavaDependencies took %s", elapsed)
	}
	if !strings.Contains(out.String(), "synced 1 jar") {
		t.Errorf("mx output not forwarded: %q", out.String())
	}
}

// Control: a failing mx is still a failure — the orphan tolerance must not
// swallow a real exit status.
func TestSyncJavaDependencies_FailingMxStillFails(t *testing.T) {
	shortOrphanWait(t)
	fake := orphanFake(t, "echo resolution failed; exit 2")
	err := SyncJavaDependencies(filepath.Join(t.TempDir(), "App.mpr"), fake, "11.12.5", nil)
	if err == nil || !strings.Contains(err.Error(), "resolution failed") {
		t.Fatalf("err = %v, want the mx failure with its output", err)
	}
}
