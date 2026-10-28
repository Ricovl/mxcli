// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1514 case B (upstream mendixlabs/mxcli#1604): `check -p` on a test
// file whose microflow a setup script has not created yet reported only
// "microflow not found". The way to check such a file exists — pass the setup
// script first, as a script set, and each file resolves against what the files
// before it write (mendixlabs/mxcli#1355) — but neither the message nor
// `mxcli help check` said it applies to test files, which create nothing of
// their own and so always depend on something else for their targets.
//
// Controls: the set form passes (so the hint points at something that works),
// and a test file whose references resolve gets no hint.
func TestCheck_TestFileMissingTargetPointsAtTheScriptSet(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}

	scripts := t.TempDir()
	setup := writeScript(t, scripts, "setup.mdl", `mdl 1;
create module Probe;
create microflow Probe.SUB_Make () returns String as $r begin return 'a;b'; end;
`)
	testFile := writeScript(t, scripts, "missing.test.mdl", `mdl 1;
/**
 * @test later
 * @expect $r = 'a;b'
 */
$r = call microflow Probe.SUB_Make();
/
`)

	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{testFile}) })
	if code == 0 || !strings.Contains(out, "microflow not found: Probe.SUB_Make") {
		t.Fatalf("the missing microflow was not reported (exit %d):\n%s", code, out)
	}
	for _, want := range []string{"a test file creates nothing", "mxcli check <setup script> " + testFile} {
		if !strings.Contains(out, want) {
			t.Errorf("the not-found report does not say %q, so the script-set form stays undiscovered:\n%s", want, out)
		}
	}

	// Control: the form the hint names works.
	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{setup, testFile}) })
	if code != 0 {
		t.Errorf("control: the setup script and the test file as one set failed (exit %d):\n%s", code, out)
	}

	// Control: nothing missing, nothing to hint at.
	ok := writeScript(t, scripts, "ok.test.mdl", "mdl 1;\n/**\n * @test fine\n * @expect $a = 1\n */\ndeclare $a Integer = 1;\n/\n")
	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{ok}) })
	if code != 0 || strings.Contains(out, "a test file creates nothing") {
		t.Errorf("control: a test file whose references resolve got the hint (exit %d):\n%s", code, out)
	}
}
