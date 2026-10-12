// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A check of several files ends on a verdict for the whole set
// (mendixlabs/mxcli#1431). It used to end on the last file's own
// "Check passed!", so a failing file followed by a passing one read as a pass
// to anything looking at the tail of the log; only the exit code said 1.
func TestCheck_ScriptSetEndsOnOverallVerdict(t *testing.T) {
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	scripts := t.TempDir()
	bad := writeScript(t, scripts, "bad.mdl", "create persistent entity M.E ( Name: String(100) \n")
	good := writeScript(t, scripts, "good.mdl", "create persistent entity M.F ( Name: String(100) );\n")

	lastLine := func(out string) string {
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		return strings.TrimSpace(lines[len(lines)-1])
	}

	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{bad, good}) })
	if code == 0 {
		t.Fatalf("a set with a failing file exited 0:\n%s", out)
	}
	if want := "1 of 2 files failed: " + bad; lastLine(out) != want {
		t.Errorf("last line = %q, want %q\n%s", lastLine(out), want, out)
	}

	// Control: a set that passes says so for the whole set.
	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{good, good}) })
	if code != 0 {
		t.Fatalf("a passing set exited %d:\n%s", code, out)
	}
	if want := "All 2 files passed"; lastLine(out) != want {
		t.Errorf("last line = %q, want %q\n%s", lastLine(out), want, out)
	}
}
