// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunState_RoundTripIsPrivateAndMissingIsNil(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if st, err := ReadRunState(mpr); err != nil || st != nil {
		t.Fatalf("missing state: got %v, %v; want nil, nil", st, err)
	}
	in := &RunState{Project: mpr, PID: 42, Phase: PhaseRunning, URL: "http://127.0.0.1:8080/", Args: []string{"run", "--local", "--db-password", "s3cret"}}
	if err := WriteRunState(mpr, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadRunState(mpr)
	if err != nil || out == nil || out.PID != 42 || out.URL != in.URL {
		t.Fatalf("round trip: got %+v, %v", out, err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(RunStatePath(mpr))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("state file mode %v, want 0600 (its args can carry a password)", fi.Mode().Perm())
		}
	}
	// No temp file left behind by the atomic rename.
	entries, _ := os.ReadDir(filepath.Dir(RunStatePath(mpr)))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

// fakeClock returns increasing times one second apart.
func fakeClock(start time.Time) func() time.Time {
	t := start
	return func() time.Time { t = t.Add(time.Second); return t }
}

func newTestRecorder(t *testing.T) (*RunStateRecorder, *[]RunState) {
	t.Helper()
	var published []RunState
	r := newRunStateRecorder("/p/App.mpr", 7, true, true, []string{"run"}, "/p/.mxcli/run.log",
		fakeClock(time.Unix(1000, 0)), func(_ string, s *RunState) error { published = append(published, *s); return nil })
	return r, &published
}

func TestRunStateRecorder_PublishesEveryPhase(t *testing.T) {
	r, published := newTestRecorder(t)
	src1 := time.Unix(500, 0)
	r.Ports(8080, 8090, 6543, "/p/.mxcli/runtime.log")
	r.BuildStarted(1, src1)
	if s := r.State(); s.Phase != PhaseStarting || !s.Building || s.Gen != 1 {
		t.Fatalf("after boot build start: %+v", s)
	}
	r.Ready("http://127.0.0.1:8080/", BuildOutcome{Gen: 1, OK: true, Action: "boot", Source: src1})
	s := r.State()
	if s.Phase != PhaseRunning || s.Building || s.Last == nil || s.Last.Gen != 1 || s.URL == "" {
		t.Fatalf("after ready: %+v", s)
	}
	src2 := time.Unix(600, 0)
	r.BuildStarted(2, src2)
	r.BuildFinished(BuildOutcome{Gen: 2, OK: false, Action: "build", Message: "Build failed", Source: src2})
	s = r.State()
	if s.Building || s.Last.Gen != 2 || s.Last.OK || s.Last.At.IsZero() {
		t.Fatalf("after failed build: %+v", s.Last)
	}
	r.Exit(nil)
	if s := r.State(); s.Phase != PhaseStopped || s.Error != "" || s.Ended.IsZero() {
		t.Fatalf("after a requested stop: %+v", s)
	}
	if len(*published) != 7 {
		t.Errorf("published %d states, want one per transition (7)", len(*published))
	}
}

func TestRunStateRecorder_ExitBeforeReadyIsAFailedBoot(t *testing.T) {
	r, _ := newTestRecorder(t)
	r.Exit(errors.New("initial build failed: CE0001"))
	if s := r.State(); s.Phase != PhaseFailed || !strings.Contains(s.Error, "CE0001") {
		t.Fatalf("got %+v", s)
	}
	r2, _ := newTestRecorder(t)
	r2.Ready("u", BuildOutcome{Gen: 1, OK: true})
	r2.Exit(errors.New("the app stopped"))
	if s := r2.State(); s.Phase != PhaseStopped || s.Error == "" {
		t.Fatalf("a runtime that died after ready is stopped-with-error, got %+v", s)
	}
}

func TestRunStateRecorder_NilIsANoOp(t *testing.T) {
	var r *RunStateRecorder
	r.Ports(1, 2, 3, "")
	r.BuildStarted(1, time.Now())
	r.BuildFinished(BuildOutcome{})
	r.Ready("", BuildOutcome{})
	r.Exit(nil)
}

// TestEvaluateWait_ChangeAppliedBeforeWaitIsNotMissed is the race `run wait`
// exists to close: the agent writes, the watch loop applies the change, and only
// then does `wait` start. A wait that blocks for "the next generation" hangs
// here until its timeout, because the change it is waiting for has already
// happened.
func TestEvaluateWait_ChangeAppliedBeforeWaitIsNotMissed(t *testing.T) {
	change := time.Unix(1000, 0)
	st := &RunState{Phase: PhaseRunning, Watch: true, Gen: 3,
		Last: &BuildOutcome{Gen: 3, OK: true, Action: "reload", Source: change}}
	v, o := EvaluateWait(st, true, change, 0)
	if v != WaitDone || o == nil || o.Gen != 3 {
		t.Fatalf("a change already applied must be reported at once; got verdict %v, outcome %+v", v, o)
	}
}

func TestEvaluateWait(t *testing.T) {
	t0 := time.Unix(1000, 0)
	older, newer := t0.Add(-time.Second), t0.Add(time.Second)
	running := func(watch bool, last *BuildOutcome, building bool) *RunState {
		return &RunState{Phase: PhaseRunning, Watch: watch, Last: last, Building: building}
	}
	cases := []struct {
		name  string
		st    *RunState
		alive bool
		since int
		want  WaitVerdict
	}{
		{"no state", nil, false, 0, WaitNotRunning},
		{"boot failed", &RunState{Phase: PhaseFailed}, false, 0, WaitBootFailed},
		{"stale: process gone", running(true, &BuildOutcome{Source: older}, false), false, 0, WaitNotRunning},
		{"stopped", &RunState{Phase: PhaseStopped}, true, 0, WaitNotRunning},
		{"still booting", &RunState{Phase: PhaseStarting}, true, 0, WaitPending},
		{"change not yet built", running(true, &BuildOutcome{Gen: 2, Source: older}, false), true, 0, WaitPending},
		{"change being built", running(true, &BuildOutcome{Gen: 2, Source: older}, true), true, 0, WaitPending},
		{"change built (failed)", running(true, &BuildOutcome{Gen: 3, OK: false, Source: newer}, false), true, 0, WaitDone},
		{"nothing changed since", running(true, &BuildOutcome{Gen: 1, OK: true, Action: "boot", Source: t0}, false), true, 0, WaitDone},
		{"not watching, change on disk", running(false, &BuildOutcome{Gen: 1, Source: older}, false), true, 0, WaitNotWatching},
		{"since: not yet", running(true, &BuildOutcome{Gen: 3, Source: newer}, false), true, 3, WaitPending},
		{"since: newer gen", running(true, &BuildOutcome{Gen: 4, Source: newer}, false), true, 3, WaitDone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := EvaluateWait(c.st, c.alive, t0, c.since)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReadyVerdict(t *testing.T) {
	for _, c := range []struct {
		st    *RunState
		alive bool
		want  WaitVerdict
	}{
		{nil, false, WaitNotRunning},
		{&RunState{Phase: PhaseStarting}, true, WaitPending},
		{&RunState{Phase: PhaseStarting}, false, WaitNotRunning},
		{&RunState{Phase: PhaseRunning}, true, WaitDone},
		{&RunState{Phase: PhaseFailed}, false, WaitBootFailed},
	} {
		if got := ReadyVerdict(c.st, c.alive); got != c.want {
			t.Errorf("%+v alive=%v: got %v, want %v", c.st, c.alive, got, c.want)
		}
	}
}

func TestFormatOutcome(t *testing.T) {
	ok := FormatOutcome(&BuildOutcome{Gen: 3, OK: true, Action: "reload", DurationMs: 1440}, 10)
	if ok != "applied: build #3 via reload in 1.4s\n" {
		t.Errorf("success line: %q", ok)
	}
	rs := FormatOutcome(&BuildOutcome{Gen: 4, OK: true, Action: "restart", DurationMs: 22000}, 10)
	if !strings.Contains(rs, "sessions dropped") {
		t.Errorf("a restart must say sessions were dropped: %q", rs)
	}
	var errs []string
	for i := 0; i < 15; i++ {
		errs = append(errs, "[CE0109] something wrong")
	}
	fail := FormatOutcome(&BuildOutcome{Gen: 5, Action: "build", Message: "Build failed\nmore", Errors: errs}, 3)
	lines := strings.Split(strings.TrimRight(fail, "\n"), "\n")
	if lines[0] != "failed: build #5 (build): Build failed" || len(lines) != 5 || !strings.Contains(lines[4], "more in the run log") {
		t.Errorf("failure output not capped at 3 error lines:\n%s", fail)
	}
}

func TestBuildErrorLines_PrefersStructuredErrorsThenServeDetails(t *testing.T) {
	b := &BuildResult{Status: "Failure", Message: "Build failed", Problems: BuildProblems{Problems: []BuildProblem{
		{Severity: "Warning", Message: "ignored"},
		{Severity: "Error", ErrorCode: "CE0109", Message: "No attribute selected", Locations: []BuildLocation{{Module: "M", Document: "Page 'P'"}}},
	}}}
	got := buildErrorLines(b, "")
	if len(got) != 1 || !strings.HasPrefix(got[0], "[CE0109] No attribute selected — at M") {
		t.Errorf("structured errors: %q", got)
	}
	theme := &BuildResult{Status: "Failure", Message: "An error occurred while compiling Theme files",
		Raw: []byte(`{"problems":{"errors":[{"message":"An error occurred while compiling Theme files","details":"Error: Undefined variable.\n  main.scss 4:18"}],"problems":[]}}`)}
	got = buildErrorLines(theme, "  This is not a problem with your model")
	if len(got) != 2 || !strings.Contains(got[0], "Undefined variable") || !strings.Contains(got[1], "not a problem with your model") {
		t.Errorf("serve error details + hint: %q", got)
	}
}
