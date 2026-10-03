// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withBinaryVersion sets the running binary's version for one test.
func withBinaryVersion(t *testing.T, version, built string) {
	t.Helper()
	oldV, oldB := Version, BuildTime
	Version, BuildTime = version, built
	t.Cleanup(func() { Version, BuildTime = oldV, oldB })
}

// versionOrderCases is shared by the Go comparison and the bootstrap script's
// copy of it, so the two cannot disagree about what "older" means.
var versionOrderCases = []struct {
	name            string
	have, haveBuilt string
	want, wantBuilt string
	older           bool
}{
	{"release older", "v0.24.0", "", "v0.25.0", "", true},
	{"release older by minor across digits", "v0.9.0", "", "v0.10.0", "", true},
	{"release older by patch", "v0.24.0", "", "v0.24.1", "", true},
	{"release equal", "v0.24.0", "", "v0.24.0", "", false},
	{"release newer", "v0.25.0", "", "v0.24.0", "", false},
	{"release major beats minor", "v1.0.0", "", "v0.99.0", "", false},
	{"nightly older", "nightly-20261001-aaaaaaa", "", "nightly-20261002-4ba1495f2", "", true},
	{"nightly same day", "nightly-20261002-aaaaaaa", "", "nightly-20261002-4ba1495f2", "", false},
	{"nightly newer", "nightly-20261003-aaaaaaa", "", "nightly-20261002-4ba1495f2", "", false},
	{"release built before nightly", "v0.24.0", "2026-06-01T10:00:00Z", "nightly-20261002-4ba1495f2", "", true},
	{"release built after nightly", "v0.25.0", "2026-10-05T10:00:00Z", "nightly-20261002-4ba1495f2", "", false},
	{"nightly before release build", "nightly-20260601-aaaaaaa", "", "v0.25.0", "2026-10-05T10:00:00Z", true},
	{"release without build time vs nightly", "v0.24.0", "", "nightly-20261002-4ba1495f2", "", false},
	{"dev binary never older", "v0.24.0-888-g4ba1495f2", "2026-01-01T00:00:00Z", "v0.25.0", "", false},
	{"dirty binary never older", "v0.24.0-dirty", "", "v0.25.0", "", false},
	{"unknown binary never older", "unknown", "", "v0.25.0", "", false},
	{"dev stamp never newer", "v0.24.0", "", "v0.24.0-914-g5d6927a8e", "", false},
}

func TestBinaryOlderThan(t *testing.T) {
	for _, c := range versionOrderCases {
		t.Run(c.name, func(t *testing.T) {
			got := binaryOlderThan(c.have, c.haveBuilt, toolingStamp{Version: c.want, Built: c.wantBuilt})
			if got != c.older {
				t.Errorf("binaryOlderThan(%q, %q, %q/%q) = %v, want %v", c.have, c.haveBuilt, c.want, c.wantBuilt, got, c.older)
			}
		})
	}
}

// The stamp is rewritten only when the version changes: the sync runs on
// every session start, and a "written" date that moved daily would dirty the
// working tree every day.
func TestWriteToolingStamp_RewritesOnlyOnVersionChange(t *testing.T) {
	dir := t.TempDir()
	withBinaryVersion(t, "v0.25.0", "2026-10-01T00:00:00Z")

	if changed, err := writeToolingStamp(dir); err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v, want a write", changed, err)
	}
	s := readToolingStamp(dir)
	if s == nil || s.Version != "v0.25.0" || s.Built != "2026-10-01T00:00:00Z" || s.Written == "" {
		t.Fatalf("stamp = %+v", s)
	}
	if changed, _ := writeToolingStamp(dir); changed {
		t.Error("second write with the same version changed the stamp")
	}
	withBinaryVersion(t, "v0.26.0", "")
	if changed, _ := writeToolingStamp(dir); !changed {
		t.Error("a new version did not rewrite the stamp")
	}
	if got := readToolingStamp(dir).Version; got != "v0.26.0" {
		t.Errorf("stamp version = %q, want v0.26.0", got)
	}
}

// The reported skew: tooling written by a newer mxcli, opened by an older
// binary. The warning names both versions and how to update, and appears once
// however many times the check runs.
func TestWarnIfToolingNewer_OnceWithBothVersions(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	withBinaryVersion(t, "v0.25.0", "")
	if _, err := writeToolingStamp(dir); err != nil {
		t.Fatal(err)
	}

	toolingWarnOnce = sync.Once{}
	t.Cleanup(func() { toolingWarnOnce = sync.Once{} })

	// Control: the binary that wrote the stamp is silent.
	var buf bytes.Buffer
	warnIfToolingNewer(&buf, mpr)
	if buf.Len() != 0 {
		t.Fatalf("same version warned: %s", buf.String())
	}

	withBinaryVersion(t, "v0.24.0", "")
	warnIfToolingNewer(&buf, mpr)
	warnIfToolingNewer(&buf, dir) // a directory -p is accepted too
	out := buf.String()
	for _, want := range []string{"v0.25.0", "v0.24.0", "setup mxcli --tag v0.25.0", bootstrapScriptName} {
		if !strings.Contains(out, want) {
			t.Errorf("warning lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "Warning:"); n != 1 {
		t.Errorf("warned %d times, want once:\n%s", n, out)
	}
}

func TestWarnIfToolingNewer_DevBuildIsSilent(t *testing.T) {
	dir := t.TempDir()
	withBinaryVersion(t, "v0.25.0", "")
	if _, err := writeToolingStamp(dir); err != nil {
		t.Fatal(err)
	}
	toolingWarnOnce = sync.Once{}
	t.Cleanup(func() { toolingWarnOnce = sync.Once{} })
	withBinaryVersion(t, "v0.24.0-888-g4ba1495f2", "")
	var buf bytes.Buffer
	warnIfToolingNewer(&buf, filepath.Join(dir, "App.mpr"))
	if buf.Len() != 0 {
		t.Errorf("a dev build warned: %s", buf.String())
	}
}
