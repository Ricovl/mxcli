// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#972 item 1: `mxcli fix hashes` on a real MPR v2 project.

func copyPedApp(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skip("testdata/pedapp not available")
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dst, "PedApp.mpr")
}

// revertOneUnit overwrites one .mxunit with different bytes, as `git checkout`
// of an older revision would, and returns its path.
func revertOneUnit(t *testing.T, mprPath string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(mprPath), "mprcontents", "*", "*", "*.mxunit"))
	if len(files) == 0 {
		t.Fatal("no .mxunit files in the copy")
	}
	f := files[len(files)/2]
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	// Same content plus a trailing byte: different hash, same unit.
	if err := os.WriteFile(f, append(b, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFixHashes_UntouchedProjectIsClean(t *testing.T) {
	mprPath := copyPedApp(t)
	var out bytes.Buffer
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("control: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "matches every unit file") {
		t.Fatalf("control not reported clean:\n%s", out.String())
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("control warned: %s", w)
	}
}

func TestFixHashes_DetectsAndRepairsRevertedUnit(t *testing.T) {
	mprPath := copyPedApp(t)
	f := revertOneUnit(t, mprPath)
	id := strings.TrimSuffix(filepath.Base(f), ".mxunit")

	var out bytes.Buffer
	err := fixHashes(mprPath, false, &out)
	if !errors.Is(err, errHashIssuesRemain) {
		t.Fatalf("verify: want errHashIssuesRemain, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "MISMATCH  "+id) || !strings.Contains(out.String(), "1 mismatch(es)") {
		t.Fatalf("mismatch not reported:\n%s", out.String())
	}
	if w := contentsHashDriftWarning(mprPath); !strings.Contains(w, "1 unit(s)") || !strings.Contains(w, "mxcli fix hashes") {
		t.Fatalf("drift warning = %q", w)
	}

	out.Reset()
	if err := fixHashes(mprPath, true, &out); err != nil {
		t.Fatalf("repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Repaired 1 ContentsHash") {
		t.Fatalf("repair not reported:\n%s", out.String())
	}

	out.Reset()
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("after repair: %v\n%s", err, out.String())
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("warned after repair: %s", w)
	}
}

func TestFixHashes_V1NotApplicable(t *testing.T) {
	src := filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project")
	files, _ := filepath.Glob(filepath.Join(src, "*.mpr"))
	if len(files) == 0 {
		t.Skip("no v1 fixture")
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	mprPath := filepath.Join(dst, filepath.Base(files[0]))
	var out bytes.Buffer
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if !strings.Contains(out.String(), "MPR v1") {
		t.Fatalf("v1 not explained:\n%s", out.String())
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("v1 warned: %s", w)
	}
}
