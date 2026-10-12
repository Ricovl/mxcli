// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// mxbuild_watchdog.go bounds a one-shot MxBuild run. Without it, `mxcli new`
// sat for 10+ minutes with Gradle spinning at 100% CPU behind a wrong
// --java-home (ChipCoV8 #6): cmd.Run() had no deadline and nothing noticed that
// the build had stopped saying anything.

const (
	// DefaultMxBuildTimeout caps a whole build. A cold first build of a large
	// project downloads Gradle dependencies, so this is generous: a false kill of
	// a slow healthy build costs more than a late one of a hung build.
	DefaultMxBuildTimeout = 30 * time.Minute
	// DefaultMxBuildStall is how long a build may go without printing a line
	// before it is presumed hung. Gradle steps can be quiet for minutes.
	DefaultMxBuildStall = 10 * time.Minute

	mxbuildTimeoutEnv = "MXCLI_MXBUILD_TIMEOUT"
	mxbuildStallEnv   = "MXCLI_MXBUILD_STALL_TIMEOUT"
)

// watchdogLimits are the two bounds; a zero disables that bound.
type watchdogLimits struct {
	Timeout time.Duration
	Stall   time.Duration
}

// mxbuildWatchdogLimits is what the one-shot MxBuild runs use; a variable so a
// test can shrink the bounds to milliseconds.
var mxbuildWatchdogLimits = mxbuildLimits

// mxbuildLimits reads the limits from the environment (Go durations such as
// "30m"; "0" disables), falling back to the defaults.
func mxbuildLimits() watchdogLimits {
	return watchdogLimits{
		Timeout: durationEnv(mxbuildTimeoutEnv, DefaultMxBuildTimeout),
		Stall:   durationEnv(mxbuildStallEnv, DefaultMxBuildStall),
	}
}

func durationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if v == "0" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return def
	}
	return d
}

// WatchdogError reports a build the watchdog killed.
type WatchdogError struct {
	Reason   string // "timed out" or "stalled"
	Limit    time.Duration
	Elapsed  time.Duration
	JavaHome string
	Tail     string // the last lines of output
	EnvVar   string // the variable that raises the limit
}

func (e *WatchdogError) Error() string {
	var b strings.Builder
	if e.Reason == "stalled" {
		fmt.Fprintf(&b, "mxbuild stalled: no output for %s (after %s), killed", e.Limit, e.Elapsed.Round(time.Second))
	} else {
		fmt.Fprintf(&b, "mxbuild timed out after %s, killed", e.Limit)
	}
	if e.JavaHome != "" {
		fmt.Fprintf(&b, "\n  Java home used: %s", e.JavaHome)
	}
	fmt.Fprintf(&b, "\n  Raise the limit with %s=<duration> (e.g. 30m; 0 disables) if the build is merely slow.", e.EnvVar)
	if e.Tail != "" {
		b.WriteString("\n  Last output:\n")
		for _, l := range strings.Split(e.Tail, "\n") {
			b.WriteString("    " + l + "\n")
		}
	} else {
		b.WriteString("\n  (mxbuild printed nothing)")
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildActivity records when a build last wrote output, and keeps the tail of
// it, across both of its streams.
type buildActivity struct {
	mu   sync.Mutex
	last time.Time
	tail []byte
}

const buildTailBytes = 16 << 10

func (a *buildActivity) saw(p []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.last = time.Now()
	a.tail = append(a.tail, p...)
	if len(a.tail) > buildTailBytes {
		a.tail = append([]byte(nil), a.tail[len(a.tail)-buildTailBytes:]...)
	}
}

func (a *buildActivity) lastWrite() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last
}

func (a *buildActivity) tailLines(n int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return lastLines(string(bytes.TrimRight(a.tail, "\n")), n)
}

// activityWriter forwards to w (which may be nil) and reports each write.
type activityWriter struct {
	act *buildActivity
	w   io.Writer
}

func (a activityWriter) Write(p []byte) (int, error) {
	a.act.saw(p)
	if a.w == nil {
		return len(p), nil
	}
	return a.w.Write(p)
}

// runWithWatchdog starts cmd with its stdout/stderr going to the writers it
// already has, and kills its whole process tree if it runs past lim.Timeout or
// prints nothing for lim.Stall. MxBuild is a wrapper around a JVM that runs
// Gradle, so killing only the direct child would leave the spinning
// grandchildren — and the pipe they hold would keep Wait from returning.
func runWithWatchdog(cmd *exec.Cmd, lim watchdogLimits, javaHome string) error {
	act := &buildActivity{last: time.Now()}
	if cmd.Stdout != nil && cmd.Stdout == cmd.Stderr {
		shared := activityWriter{act, cmd.Stdout}
		cmd.Stdout, cmd.Stderr = shared, shared
	} else {
		cmd.Stdout = activityWriter{act, cmd.Stdout}
		cmd.Stderr = activityWriter{act, cmd.Stderr}
	}
	setProcessGroup(cmd)
	// Once the tree is killed nothing should hold the pipes, but never let a
	// stray descendant turn a kill into a second hang.
	cmd.WaitDelay = 10 * time.Second

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	tick := time.Second
	if lim.Stall > 0 && lim.Stall/4 < tick {
		tick = max(lim.Stall/4, 10*time.Millisecond)
	}
	if lim.Timeout > 0 && lim.Timeout/4 < tick {
		tick = max(lim.Timeout/4, 10*time.Millisecond)
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case now := <-ticker.C:
			var werr *WatchdogError
			switch {
			case lim.Timeout > 0 && now.Sub(start) >= lim.Timeout:
				werr = &WatchdogError{Reason: "timed out", Limit: lim.Timeout, EnvVar: mxbuildTimeoutEnv}
			case lim.Stall > 0 && now.Sub(act.lastWrite()) >= lim.Stall:
				werr = &WatchdogError{Reason: "stalled", Limit: lim.Stall, EnvVar: mxbuildStallEnv}
			}
			if werr == nil {
				continue
			}
			_ = killProcessGroup(cmd.Process)
			<-done
			werr.Elapsed = time.Since(start)
			werr.JavaHome = javaHome
			werr.Tail = act.tailLines(20)
			return werr
		}
	}
}
