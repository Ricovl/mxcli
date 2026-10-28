// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// ako/mxcli#1542, expected behaviour 1, verbatim: "When the runtime stops
// (licence limit, crash, kill), `run status` says so, with the reason when the
// runtime log gives one: e.g. `stopped: trial licence time limit reached at
// 18:43:16 (runtime.log); restart with mxcli run restart`."

const licenceStoppedRuntimeLog = "\n=== runtime start 2026-10-26T14:51:02Z ===\n" +
	"2026-10-26 14:51:20.101 WARNING - LicenseService: The runtime has been started using a trial licence,\n" +
	"          the framework will be terminated when the maximum time is exceeded!\n" +
	"2026-10-26 18:43:16.412 CRITICAL - LicenseService: Maximum run time exceeded, framework is now terminating\n" +
	"\tat com.mendix.basis.license.LicenseUtil.checkRuntimeDuration(LicenseUtil.java:212)\n" +
	"2026-10-26 18:43:16.530 INFO - Core: Mendix Runtime is shutting down\n"

// stoppedRunFixture writes the runtime.log and the run state a licence stop
// leaves behind, and returns the project.
func stoppedRunFixture(t *testing.T, mutate func(*docker.RunState)) string {
	t.Helper()
	mpr := scratchProject(t)
	logPath := filepath.Join(filepath.Dir(mpr), ".mxcli", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(licenceStoppedRuntimeLog), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	st := &docker.RunState{
		Project: mpr, PID: 1 << 30, Phase: docker.PhaseStopped, Watch: true,
		URL: "http://127.0.0.1:8080/", RuntimeLog: logPath, Args: []string{"run", "--local", "--watch", "-p", mpr},
		Started: now.Add(-4 * time.Hour), Ready: now.Add(-4 * time.Hour), Ended: now.Add(-time.Minute),
		// Exactly what a licence stop recorded: the reason was never captured,
		// so only the generic first line remained.
		Error: "the app stopped: the Mendix runtime is no longer running\n" +
			"  mxcli is exiting rather than staying up over a dead app",
	}
	if mutate != nil {
		mutate(st)
	}
	if err := docker.WriteRunState(mpr, st); err != nil {
		t.Fatal(err)
	}
	return mpr
}

func TestRunStatus_RuntimeStopNamesTheReason(t *testing.T) {
	mpr := stoppedRunFixture(t, nil)
	var out bytes.Buffer
	code := runStatus(mpr, &out)
	line := strings.TrimSpace(out.String())
	if code != runExitNotRunning {
		t.Errorf("exit %d, want %d (%s)", code, runExitNotRunning, line)
	}
	for _, want := range []string{
		"stopped: trial licence time limit reached at 18:43:16 (runtime.log)",
		"mxcli run restart",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("run status does not say why the app stopped — callers blamed their "+
				"credentials instead (ako/mxcli#1542)\n got: %s\nwant: %s", line, want)
		}
	}
}

// A run killed outright never records its end; the pid is gone and the state
// still says running. The runtime log can still say why.
func TestRunStatus_KilledRunStillNamesTheLogReason(t *testing.T) {
	mpr := stoppedRunFixture(t, func(st *docker.RunState) {
		st.Phase, st.Error, st.Ended = docker.PhaseRunning, "", time.Time{}
	})
	var out bytes.Buffer
	code := runStatus(mpr, &out)
	line := strings.TrimSpace(out.String())
	if code != runExitNotRunning {
		t.Errorf("exit %d, want %d (%s)", code, runExitNotRunning, line)
	}
	for _, want := range []string{"stopped: pid", "exited without cleaning up", "trial licence time limit reached at 18:43:16"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q lacks %q", line, want)
		}
	}
}

// The reason the run recorded wins over re-reading the log, and a plain
// requested stop still says only "stopped".
func TestStatusLine_StoppedReason(t *testing.T) {
	now := time.Now()
	st := &docker.RunState{Project: "/p/App.mpr", PID: 7, Phase: docker.PhaseStopped, Ended: now,
		Error: "the app stopped: the Mendix runtime is no longer running", Reason: "last runtime error at 12:30:02: Core: db gone (runtime.log)"}
	line, code := statusLine(st, statusProbes{alive: neverAlive}, now, now)
	if code != runExitNotRunning || line != "stopped: last runtime error at 12:30:02: Core: db gone (runtime.log); restart with mxcli run restart -p /p/App.mpr" {
		t.Errorf("exit %d, line %q", code, line)
	}
	plain := &docker.RunState{Project: "/p/App.mpr", PID: 7, Phase: docker.PhaseStopped, Ended: now, RuntimeLog: "/p/.mxcli/runtime.log"}
	probes := statusProbes{alive: neverAlive, logReason: func(*docker.RunState) string { return "should not be asked" }}
	if line, _ := statusLine(plain, probes, now, now); line != "stopped" {
		t.Errorf("a requested stop must not borrow a reason from the log: %q", line)
	}
}

// Expected behaviour, and the state #1542 was found in: the app is up, the
// handshake is gone. Saying "stopped" would be false (and `run status ||
// run --detach` would then collide with the live run); saying nothing leaves
// the 403 unexplained. Running, exit 0, with the missing file named.
func TestStatusLine_RunningButHandshakeMissing(t *testing.T) {
	now := time.Now()
	st := &docker.RunState{Project: "/p/App.mpr", PID: 7, Phase: docker.PhaseRunning, Watch: true,
		URL: "http://127.0.0.1:8080/", Ready: now.Add(-time.Hour)}
	missing := statusProbes{alive: alwaysAlive, handshake: func(*docker.RunState) bool { return false }}
	line, code := statusLine(st, missing, time.Time{}, now)
	if code != runExitOK || !strings.HasPrefix(line, "running: ") {
		t.Errorf("exit %d, line %q — the app is still up", code, line)
	}
	for _, want := range []string{"run-local.json is MISSING", "oql, log and constant --apply", "mxcli run restart -p /p/App.mpr"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q lacks %q", line, want)
		}
	}
	present := statusProbes{alive: alwaysAlive, handshake: func(*docker.RunState) bool { return true }}
	if line, _ := statusLine(st, present, time.Time{}, now); strings.Contains(line, "MISSING") {
		t.Errorf("CONTROL: a published handshake must not be reported missing: %q", line)
	}
}

// Expected behaviour 3's caller side: "what a caller should do when it is
// missing (check `run status`, not credentials)". The fake admin API stands in
// for a runtime that is up but was booted with another password.
func TestDevLoopAdminHint_ForbiddenWithoutHandshakePointsAtRunStatus(t *testing.T) {
	t.Setenv("ADMIN_PORT", "")
	t.Setenv("M2EE_ADMIN_PASS", "")
	p := stoppedRunFixture(t, func(st *docker.RunState) {
		st.Reason = "trial licence time limit reached at 18:43:16 (runtime.log)"
	})
	port := fakeAdminAPI(t, "loop-secret")
	set := adminFlagsSet{port: true}
	opts := devLoopAdminOptions(p, docker.M2EEOptions{ProjectPath: p, Port: port, Direct: true}, set)
	_, err := docker.ExecuteOQL(docker.OQLOptions{Host: "127.0.0.1", Port: opts.Port, Token: opts.Token,
		ProjectPath: p, Direct: true}, "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected the admin API to refuse the default password, got %v", err)
	}
	hint := devLoopAdminHint(p, set, err)
	t.Logf("what `mxcli oql` prints:\nError: %v\n%s", err, hint)
	for _, want := range []string{
		"run-local.json is missing",
		"is NOT a\n  wrong password",
		"mxcli run status -p " + p,
		"stopped: trial licence time limit reached at 18:43:16 (runtime.log)",
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint lacks %q:\n%s", want, hint)
		}
	}

	// A stale handshake is named as such.
	if err := writeDevLoopHandshake(p, devLoopHandshake{Project: p, PID: 1 << 30, AdminPort: port}); err != nil {
		t.Fatal(err)
	}
	if hint := devLoopAdminHint(p, set, err); !strings.Contains(hint, "names pid 1073741824, which is no longer running") {
		t.Errorf("stale handshake hint:\n%s", hint)
	}

	// CONTROL: with the live run's handshake in hand there is nothing to explain.
	publishFakeLoop(t, p, port, "loop-secret")
	if hint := devLoopAdminHint(p, set, err); hint != "" {
		t.Errorf("a live handshake was used; no hint expected, got:\n%s", hint)
	}
	// Nor when the user named the endpoint themselves.
	if hint := devLoopAdminHint(p, adminFlagsSet{host: true}, err); hint != "" {
		t.Errorf("explicit --host: no hint expected, got:\n%s", hint)
	}
}

// `mxcli log` gets the same explanation; its admin errors arrive wrapped as
// unreachable, so the auth failure must survive the wrapping.
func TestLogConnectionHint_ExplainsAMissingHandshake(t *testing.T) {
	p := stoppedRunFixture(t, func(st *docker.RunState) {
		st.Reason = "trial licence time limit reached at 18:43:16 (runtime.log)"
	})
	port := fakeAdminAPI(t, "loop-secret")
	cmd := newLogFlagsCmd()
	if err := cmd.ParseFlags([]string{"-p", p, "--admin-port", strconv.Itoa(port)}); err != nil {
		t.Fatal(err)
	}
	_, err := docker.GetLogSettings(logAdminOptions(cmd))
	if err == nil || !errors.Is(err, docker.ErrAdminAuth) {
		t.Fatalf("expected an auth failure that errors.Is can still see, got %v", err)
	}
	hint := logConnectionHint(cmd, err)
	if !strings.Contains(hint, "run-local.json is missing") || !strings.Contains(hint, "trial licence time limit reached") {
		t.Errorf("log hint:\n%s", hint)
	}
}

// `constant set --apply` reads the handshake itself; its refusal quotes run
// status too.
func TestReadDevLoopHandshake_MissingQuotesRunStatus(t *testing.T) {
	p := stoppedRunFixture(t, func(st *docker.RunState) {
		st.Reason = "trial licence time limit reached at 18:43:16 (runtime.log)"
	})
	_, err := readDevLoopHandshake(p)
	if err == nil || !errors.Is(err, errNoDevLoopHandshake) ||
		!strings.Contains(err.Error(), "mxcli run status: stopped: trial licence time limit reached at 18:43:16") {
		t.Fatalf("got %v", err)
	}
}
