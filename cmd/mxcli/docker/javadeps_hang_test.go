// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#1340: the step "stops for more than 10 minutes, without any
// output". The reporter's suggestion: "show progress or a timeout instead of
// waiting silently". The fake mx prints what Gradle prints and then never
// finishes — the shape of a client attached to a wedged daemon.
func TestSyncJavaDependencies_HungSyncShowsProgressAndTimesOut(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mx")
	script := "#!/bin/sh\necho 'Starting a Gradle Daemon, 1 busy and 1 stopped Daemons could not be reused'\nsleep 30\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(syncJavaDepsTimeoutEnv, "700ms")
	old := syncJavaDepsProgressEvery
	syncJavaDepsProgressEvery = 150 * time.Millisecond
	t.Cleanup(func() { syncJavaDepsProgressEvery = old })

	var out syncBuffer
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- SyncJavaDependencies(filepath.Join(dir, "App.mpr"), fake, "11.12.4", &out) }()
	var err error
	select {
	case err = <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("SyncJavaDependencies still running after 15s, having printed only:\n%q\n"+
			"— the silent hang at 'Resolving managed Java dependencies' (ako/mxcli#1340)", out.String())
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("a 700ms bound took %s: the process tree was not killed", elapsed)
	}
	if !strings.Contains(out.String(), "still resolving") {
		t.Errorf("a long sync must say it is still working; output:\n%s", out.String())
	}
	if err == nil {
		t.Fatal("a sync killed by its bound must be an error")
	}
	var werr *WatchdogError
	if !errors.As(err, &werr) {
		t.Errorf("err = %T, want a *WatchdogError", err)
	}
	msg := err.Error()
	t.Logf("output:\n%s\nerror:\n%s", out.String(), msg)
	for _, want := range []string{
		"mx sync-java-dependencies", // the step
		"timed out",
		"Gradle daemon",        // the likely cause
		syncJavaDepsTimeoutEnv, // how to allow longer
		"could not be reused",  // the last thing mx said
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
}

// CONTROL: a sync that finishes inside the progress interval is as quiet as
// before, and its success is its own.
func TestSyncJavaDependencies_FastSyncIsQuiet(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mx")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho synced 1 jar\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := syncJavaDepsProgressEvery
	syncJavaDepsProgressEvery = 5 * time.Second
	t.Cleanup(func() { syncJavaDepsProgressEvery = old })

	var out syncBuffer
	if err := SyncJavaDependencies(filepath.Join(dir, "App.mpr"), fake, "11.12.4", &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "still resolving") {
		t.Errorf("a fast sync printed progress:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "synced 1 jar") {
		t.Errorf("mx output not forwarded:\n%s", out.String())
	}
}

// The docker build's one-shot MxBuild shares the setting; asserted on the
// environment the child actually receives.
func TestMxBuildCmd_RunsGradleWithoutADaemon(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mxbuild")
	seen := filepath.Join(dir, "gradle_opts")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s' \"$GRADLE_OPTS\" > '"+seen+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRADLE_OPTS", "-Xmx1g")
	var out syncBuffer
	if err := mxbuildCmd(fake, "/usr", dir, filepath.Join(dir, "App.mpr"), &out, &out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "-Xmx1g -Dorg.gradle.daemon=false" {
		t.Errorf("mxbuild saw GRADLE_OPTS=%q, want the user's options plus -Dorg.gradle.daemon=false", got)
	}
}
