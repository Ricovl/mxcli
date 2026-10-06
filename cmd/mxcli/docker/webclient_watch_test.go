// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"testing"
	"time"
)

func TestParseBundlerStatus(t *testing.T) {
	cases := []struct {
		line     string
		wantKind string
		wantOK   bool
	}{
		{`{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"start"}}`, "start", true},
		{`{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"success"}}`, "success", true},
		{`{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"error","error":{"message":"boom"}}}`, "error", true},
		{`Initially, there are 40 chunks`, "", false},                                               // plain log line
		{`{"protocol":"other","type":"status","payload":{"kind":"x"}}`, "", false},                  // wrong protocol
		{`{"protocol":"mx-modern-web-bundler","type":"command","payload":{"kind":"x"}}`, "", false}, // not status
		{`not json`, "", false},
	}
	for _, tc := range cases {
		s, ok := parseBundlerStatus(tc.line)
		if ok != tc.wantOK || s.kind != tc.wantKind {
			t.Errorf("parseBundlerStatus(%q) = (%q,%v), want (%q,%v)", tc.line, s.kind, ok, tc.wantKind, tc.wantOK)
		}
	}
	s, _ := parseBundlerStatus(`{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"error","error":{"message":"boom"}}}`)
	if s.message != "boom" {
		t.Errorf("error message = %q, want boom", s.message)
	}
}

// TestParseBundlerStatus_Legacy covers the runner shipped before the
// modern-web-bundler protocol (10.24, 11.6): `out(code, payload)` writes
// {"code":"START"|"SUCCESS"|"ERROR","payload":…}, the payload an object with a
// message or, when the config fails to load, a bare string. Not recognising it
// left --watch waiting the full web-client timeout for a bundle that had
// finished in 9s (nightly 2026-10-06, Mendix 10.24.24 and 11.6.8).
func TestParseBundlerStatus_Legacy(t *testing.T) {
	cases := []struct {
		line, wantKind, wantMsg string
	}{
		{`{"code":"START"}`, "start", ""},
		{`{"code":"SUCCESS"}`, "success", ""},
		{`{"code":"ERROR","payload":{"type":"GENERIC","message":"boom","fileName":null}}`, "error", "boom"},
		{`{"code":"ERROR","payload":"Error: Cannot find module rollup.config.mjs"}`, "error", "Error: Cannot find module rollup.config.mjs"},
	}
	for _, tc := range cases {
		s, ok := parseBundlerStatus(tc.line)
		if !ok || s.kind != tc.wantKind || s.message != tc.wantMsg {
			t.Errorf("parseBundlerStatus(%q) = (%q,%q,%v), want (%q,%q,true)", tc.line, s.kind, s.message, ok, tc.wantKind, tc.wantMsg)
		}
	}
	for _, line := range []string{`{"code":"OTHER"}`, `{"code":""}`, `{"payload":{}}`} {
		if s, ok := parseBundlerStatus(line); ok {
			t.Errorf("parseBundlerStatus(%q) = (%q,true), want not a status", line, s.kind)
		}
	}
}

// newTestWatcher returns a watcher with no process, drivable via applyStatus.
func newTestWatcher() *WebClientWatcher {
	return &WebClientWatcher{log: &syncBuffer{}, updated: make(chan struct{})}
}

func TestApplyStatus_Generation(t *testing.T) {
	wc := newTestWatcher()
	if wc.Generation() != 0 {
		t.Fatal("initial generation should be 0")
	}
	wc.applyStatus(bundlerStatus{kind: "start"})
	wc.applyStatus(bundlerStatus{kind: "success"})
	if wc.Generation() != 1 {
		t.Errorf("generation = %d, want 1", wc.Generation())
	}
	wc.applyStatus(bundlerStatus{kind: "start"})
	wc.applyStatus(bundlerStatus{kind: "success"})
	if wc.Generation() != 2 {
		t.Errorf("generation = %d, want 2", wc.Generation())
	}
}

func TestWaitForRebuild_Success(t *testing.T) {
	wc := newTestWatcher()
	gen := wc.Generation()
	go func() {
		time.Sleep(20 * time.Millisecond)
		wc.applyStatus(bundlerStatus{kind: "start"})
		time.Sleep(20 * time.Millisecond)
		wc.applyStatus(bundlerStatus{kind: "success"})
	}()
	rebuilt, err := wc.WaitForRebuild(gen, time.Second, 5*time.Second)
	if err != nil || !rebuilt {
		t.Fatalf("WaitForRebuild = (%v,%v), want (true,nil)", rebuilt, err)
	}
}

func TestWaitForRebuild_AlreadyAdvanced(t *testing.T) {
	wc := newTestWatcher()
	gen := wc.Generation()
	wc.applyStatus(bundlerStatus{kind: "start"})
	wc.applyStatus(bundlerStatus{kind: "success"})
	rebuilt, err := wc.WaitForRebuild(gen, time.Second, 5*time.Second)
	if err != nil || !rebuilt {
		t.Errorf("expected immediate (true,nil), got (%v,%v)", rebuilt, err)
	}
}

func TestWaitForRebuild_SettlesOutNoRebuild(t *testing.T) {
	wc := newTestWatcher()
	start := time.Now()
	// No bundle ever starts (the change didn't hit rollup's graph) -> false, no hang.
	rebuilt, err := wc.WaitForRebuild(wc.Generation(), 150*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForRebuild: %v", err)
	}
	if rebuilt {
		t.Error("expected rebuilt=false when no bundle starts")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("should settle out near the settle window, not hang")
	}
}

func TestWaitForRebuild_Error(t *testing.T) {
	wc := newTestWatcher()
	gen := wc.Generation()
	go func() {
		time.Sleep(10 * time.Millisecond)
		wc.applyStatus(bundlerStatus{kind: "start"})
		time.Sleep(10 * time.Millisecond)
		wc.applyStatus(bundlerStatus{kind: "error", message: "syntax error in widget"})
	}()
	if _, err := wc.WaitForRebuild(gen, time.Second, 5*time.Second); err == nil {
		t.Fatal("expected an error when the bundle fails")
	}
}
