// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// bogusOneShot is a one-shot re-bundle that fails visibly (no mxbuild there), so
// a test can tell that the recovery branch ran.
func bogusOneShot(deployDir string) func() error {
	return func() error {
		return BuildWebClient(WebClientOptions{DeployDir: deployDir, MxBuildPath: "/nonexistent/mxbuild"})
	}
}

// bundlerPool tracks fake bundlers, so a test can assert how many ran at once.
type bundlerPool struct {
	mu      sync.Mutex
	live    int
	maxLive int
	started int
	// onStart runs when a fake bundler starts — its "first build".
	onStart func()
}

func (p *bundlerPool) start() (clientBundler, error) {
	p.mu.Lock()
	p.live++
	p.started++
	if p.live > p.maxLive {
		p.maxLive = p.live
	}
	p.mu.Unlock()
	if p.onStart != nil {
		p.onStart()
	}
	return &fakeBundler{pool: p, gen: 1}, nil
}

func (p *bundlerPool) liveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.live
}

type fakeBundler struct {
	pool    *bundlerPool
	gen     int
	exited  bool
	stopped bool
	waitErr error // WaitForRebuild's error while the process is alive
}

func (f *fakeBundler) Generation() int { return f.gen }
func (f *fakeBundler) WaitForRebuild(int, time.Duration, time.Duration) (bool, error) {
	if f.exited {
		return false, errors.New("web client watcher exited:\nboom")
	}
	if f.waitErr != nil {
		return false, f.waitErr
	}
	return true, nil
}
func (f *fakeBundler) Exited() bool { return f.exited }
func (f *fakeBundler) Log() string  { return "runner log line\n" }

// die simulates the bundler process exiting on its own.
func (f *fakeBundler) die() {
	if !f.exited {
		f.exited = true
		f.pool.mu.Lock()
		f.pool.live--
		f.pool.mu.Unlock()
	}
}

func (f *fakeBundler) Stop() error {
	f.stopped = true
	f.die()
	return nil
}

func newFakeSupervisor(pool *bundlerPool, out io.Writer) (*bundlerSupervisor, *fakeBundler) {
	first, _ := pool.start()
	return &bundlerSupervisor{start: pool.start, out: out, cur: first, want: true, now: time.Now}, first.(*fakeBundler)
}

// The reported symptom: once the bundler had exited, every later change failed
// with "watcher exited" and nothing restarted it. EnsureAlive must start a fresh
// one, and say so.
func TestBundlerSupervisor_RestartsExitedBundler(t *testing.T) {
	pool := &bundlerPool{}
	var out bytes.Buffer
	sup, first := newFakeSupervisor(pool, &out)

	if restarted, err := sup.EnsureAlive(); err != nil || restarted {
		t.Fatalf("live bundler: EnsureAlive = (%v, %v), want (false, nil)", restarted, err)
	}
	first.die()
	restarted, err := sup.EnsureAlive()
	if err != nil || !restarted {
		t.Fatalf("exited bundler: EnsureAlive = (%v, %v), want (true, nil)", restarted, err)
	}
	if pool.started != 2 || pool.liveCount() != 1 {
		t.Fatalf("started=%d live=%d, want a second bundler and exactly one live", pool.started, pool.liveCount())
	}
	if sup.cur == clientBundler(first) || sup.cur.Exited() {
		t.Fatal("the supervisor still holds the dead bundler")
	}
	if !strings.Contains(out.String(), "exited unexpectedly") || !strings.Contains(out.String(), "runner log line") {
		t.Fatalf("restart not reported with the dead bundler's log:\n%s", out.String())
	}
}

// A bundler that cannot start must not be retried on every change: after a
// failure the next attempt waits out a growing backoff.
func TestBundlerSupervisor_BackoffAfterFailedStart(t *testing.T) {
	now := time.Unix(1000, 0)
	attempts := 0
	fail := true
	pool := &bundlerPool{}
	sup := &bundlerSupervisor{
		start: func() (clientBundler, error) {
			attempts++
			if fail {
				return nil, errors.New("node crashed")
			}
			return pool.start()
		},
		out: io.Discard, want: true, now: func() time.Time { return now },
		cur: &fakeBundler{pool: pool, exited: true},
	}

	if _, err := sup.EnsureAlive(); err == nil || !strings.Contains(err.Error(), "node crashed") {
		t.Fatalf("first restart: want the start error, got %v", err)
	}
	if _, err := sup.EnsureAlive(); err == nil || !strings.Contains(err.Error(), "next attempt in 2s") {
		t.Fatalf("within backoff: want a 'next attempt' error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("restart retried inside the backoff window: %d attempts", attempts)
	}
	now = now.Add(3 * time.Second)
	fail = false
	if restarted, err := sup.EnsureAlive(); err != nil || !restarted {
		t.Fatalf("after backoff: EnsureAlive = (%v, %v), want (true, nil)", restarted, err)
	}
	if attempts != 2 || sup.failures != 0 {
		t.Fatalf("attempts=%d failures=%d, want 2 and a reset counter", attempts, sup.failures)
	}
}

func TestBundlerRestartBackoff(t *testing.T) {
	want := map[int]time.Duration{0: 0, 1: 2 * time.Second, 2: 4 * time.Second, 3: 8 * time.Second, 10: time.Minute}
	for n, d := range want {
		if got := bundlerRestartBackoff(n); got != d {
			t.Errorf("bundlerRestartBackoff(%d) = %s, want %s", n, got, d)
		}
	}
}

// A recovery re-bundle under --watch must replace the bundler, never run beside it.
func TestBundlerSupervisor_RebundleNeverOverlaps(t *testing.T) {
	pool := &bundlerPool{}
	sup, _ := newFakeSupervisor(pool, io.Discard)
	for i := 0; i < 3; i++ {
		if err := sup.Rebundle(); err != nil {
			t.Fatal(err)
		}
	}
	if pool.maxLive != 1 {
		t.Fatalf("%d bundlers ran at once, want 1", pool.maxLive)
	}
}

// The concurrency defect end to end: a page missing from the bundle under
// --watch must be recovered through the supervisor. A one-shot BuildWebClient
// while the incremental bundler is live puts two rollups on web/dist (#971).
func TestEnsureClientServed_WatchModeRebundleIsExclusive(t *testing.T) {
	deploy := pagesDeployment(t, []string{"A.js", "B.js"}, []string{"A.js"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := &bundlerPool{}
	sup, _ := newFakeSupervisor(pool, io.Discard)
	// A fresh bundler's first build emits every page.
	pool.onStart = func() {
		_ = os.WriteFile(filepath.Join(deploy, "web", "dist", "pages", "B.js"), []byte("//"), 0o600)
	}

	oneShotWhileLive := 0
	oneShots := 0
	old := oneShotWebClientBuild
	oneShotWebClientBuild = func(WebClientOptions) error {
		oneShots++
		if pool.liveCount() > 0 {
			oneShotWhileLive++
		}
		pool.onStart()
		return nil
	}
	defer func() { oneShotWebClientBuild = old }()

	rebundle := clientRebundler(sup, WebClientOptions{DeployDir: deploy})
	if err := ensureClientServed(deploy, srv.URL+"/", rebundle, io.Discard); err != nil {
		t.Fatalf("ensureClientServed: %v", err)
	}
	if oneShotWhileLive > 0 {
		t.Fatalf("a one-shot bundle ran %d time(s) while the incremental bundler was live", oneShotWhileLive)
	}
	if pool.maxLive != 1 || pool.started != 2 {
		t.Fatalf("maxLive=%d started=%d, want the bundler replaced (2 starts, 1 at a time)", pool.maxLive, pool.started)
	}

	// Control: without an incremental bundler the one-shot is the right tool.
	deploy2 := pagesDeployment(t, []string{"A.js", "B.js"}, []string{"A.js"})
	pool.onStart = func() {
		_ = os.WriteFile(filepath.Join(deploy2, "web", "dist", "pages", "B.js"), []byte("//"), 0o600)
	}
	none := newBundlerSupervisor(nil, nil, io.Discard)
	if err := ensureClientServed(deploy2, srv.URL+"/", clientRebundler(none, WebClientOptions{DeployDir: deploy2}), io.Discard); err != nil {
		t.Fatalf("ensureClientServed without a bundler: %v", err)
	}
	if oneShots != 1 {
		t.Fatalf("without a bundler the one-shot should run once, ran %d", oneShots)
	}
}

// A nil supervisor bundler (Mendix 11.14+, classic client) is inert.
func TestBundlerSupervisor_NoBundlerIsNoop(t *testing.T) {
	sup := newBundlerSupervisor(nil, func() (*WebClientWatcher, error) {
		t.Fatal("start must not be called")
		return nil, nil
	}, io.Discard)
	if sup.Wanted() || sup.Generation() != 0 {
		t.Fatal("a supervisor without a bundler reports one")
	}
	if r, err := sup.EnsureAlive(); r || err != nil {
		t.Fatalf("EnsureAlive = (%v, %v)", r, err)
	}
	if err := sup.Restart(); err != nil {
		t.Fatal(err)
	}
	sup.Stop()
}

// Stop must not return while the bundler process is still running: the
// supervisor's restart starts the next bundler as soon as Stop returns. A guard,
// not a regression test — the earlier double-Wait Stop also blocked until exit
// on this Go version, so this passes against it too.
func TestWebClientWatcher_StopWaitsForExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group signalling is unix-only")
	}
	// Takes ~1s to exit on SIGTERM, the way node finishes a bundle in flight.
	cmd := exec.Command("sh", "-c", `trap 'sleep 1; exit 0' TERM; echo ready; while :; do sleep 0.05; done`)
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	wc, err := launchWatcher(cmd, &syncBuffer{}, stdout)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the shell install its trap
	if wc.Exited() {
		t.Fatal("process exited before Stop")
	}
	_ = wc.Stop()
	if !wc.Exited() {
		t.Fatal("Stop returned while the bundler process was still running")
	}
}

func TestWebClientTimeout(t *testing.T) {
	t.Setenv(webClientTimeoutEnv, "")
	if got := webClientTimeout(0); got != defaultWebClientTimeout {
		t.Errorf("default = %s", got)
	}
	t.Setenv(webClientTimeoutEnv, "12m")
	if got := webClientTimeout(0); got != 12*time.Minute {
		t.Errorf("env = %s, want 12m", got)
	}
	if got := webClientTimeout(3 * time.Second); got != 3*time.Second {
		t.Errorf("explicit = %s, want it to win over the env", got)
	}
	t.Setenv(webClientTimeoutEnv, "soon")
	if got := webClientTimeout(0); got != defaultWebClientTimeout {
		t.Errorf("unparsable env = %s, want the default", got)
	}
}

// A timed-out bundle used to say only "timed out after 5m0s". It must name the
// knob and show the bundler's own log, which is where the stall is visible.
func TestBuildWebClient_TimeoutShowsLogTail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake node is a shell script")
	}
	deploy := t.TempDir()
	mustWrite := func(p, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(deploy, "web", "rollup.config.mjs"), "export default {};", 0o644)
	var log strings.Builder
	for i := 1; i <= 40; i++ {
		log.WriteString("line ")
		log.WriteString(strings.Repeat("x", i%3))
		log.WriteString("\n")
	}
	log.WriteString("INFO Bundling started\n")
	mustWrite(filepath.Join(deploy, "log", "web-client-build.log"), log.String(), 0o644)

	mx := t.TempDir()
	mustWrite(filepath.Join(mx, "tools", "node", "rollup-runner.mjs"), "", 0o644)
	mustWrite(filepath.Join(mx, "tools", "node", "node"), "#!/bin/sh\nexec sleep 30\n", 0o755)

	err := BuildWebClient(WebClientOptions{
		DeployDir: deploy, MxBuildPath: filepath.Join(mx, "mxbuild"), Timeout: 300 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"timed out after 300ms", "--web-client-timeout", webClientTimeoutEnv, "INFO Bundling started", "last 30 line(s)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("timeout error lacks %q:\n%s", want, msg)
		}
	}
}

func TestSessionNotice(t *testing.T) {
	if sessionNotice(ActionReload) != "" {
		t.Error("a reload keeps sessions; no notice")
	}
	if n := sessionNotice(ActionRestart); !strings.Contains(n, "sessions were dropped") {
		t.Errorf("restart notice = %q", n)
	}
}

// Both shapes the reported loop died of: an incremental build that failed on the
// source the serve build was rewriting (measured on 11.13 after adding an entity:
// ENOTDIR on web/pages/<Page>.js/package.json, then the same on the next change),
// and a bundler that had exited. A fresh bundler must deliver the change.
func TestBundlerSupervisor_AwaitRebuildRecovers(t *testing.T) {
	cases := map[string]func(*fakeBundler){
		"failed incremental build": func(f *fakeBundler) {
			f.waitErr = errors.New("web client build failed: (plugin commonjs--resolver) Error: ENOTDIR: not a directory\nstack")
		},
		"exited bundler": func(f *fakeBundler) { f.die() },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			pool := &bundlerPool{}
			sup, first := newFakeSupervisor(pool, io.Discard)
			breakIt(first)
			var errOut bytes.Buffer
			rebuilt, err := sup.AwaitRebuild(0, time.Millisecond, time.Millisecond, &errOut)
			if err != nil || !rebuilt {
				t.Fatalf("AwaitRebuild = (%v, %v), want the change delivered by a fresh bundler", rebuilt, err)
			}
			if pool.started != 2 || pool.maxLive != 1 {
				t.Fatalf("started=%d maxLive=%d, want one replacement and never two at once", pool.started, pool.maxLive)
			}
			if !strings.Contains(errOut.String(), "incremental web client rebuild failed") || strings.Contains(errOut.String(), "stack") {
				t.Fatalf("the incremental failure is not reported on one line:\n%s", errOut.String())
			}
		})
	}
}

func TestBundlerSupervisor_AwaitRebuildFreshFailsToo(t *testing.T) {
	pool := &bundlerPool{}
	sup, first := newFakeSupervisor(pool, io.Discard)
	first.waitErr = errors.New("web client build failed: syntax error")
	sup.start = func() (clientBundler, error) { return nil, errors.New("initial web client build failed: syntax error") }
	if _, err := sup.AwaitRebuild(0, time.Millisecond, time.Millisecond, io.Discard); err == nil || !strings.Contains(err.Error(), "fresh bundler failed too") {
		t.Fatalf("a genuine build error must surface, got %v", err)
	}
}

func TestBundlerSupervisor_AwaitRebuildHealthyNoRestart(t *testing.T) {
	pool := &bundlerPool{}
	sup, _ := newFakeSupervisor(pool, io.Discard)
	if rebuilt, err := sup.AwaitRebuild(0, time.Millisecond, time.Millisecond, io.Discard); err != nil || !rebuilt {
		t.Fatalf("AwaitRebuild = (%v, %v)", rebuilt, err)
	}
	if pool.started != 1 {
		t.Fatalf("a healthy rebuild restarted the bundler (%d starts)", pool.started)
	}
}
