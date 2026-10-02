// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#677: `check -p` on a .test.mdl reported "module not found: MxTest"
// once per test, so a test file could never pass check. The rendering wraps
// each block in a microflow of the runner's own MxTest module — the module
// every generated runner script creates before anything else — and the
// reference pass resolved it against a project that does not have it.
//
// The control is a block with a reference that really is missing: the file is
// still checked, not exempted.
func TestCheck_TestFileResolvesTheRunnersModule(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	// The subcommand sees the root's persistent flags once they are merged,
	// which cobra otherwise does only when it executes.
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()

	const tests = "/** @test one */\nDECLARE $a Boolean = true;\n/\n\n/** @test two */\nDECLARE $b Integer = 1;\n/\n"
	for _, header := range []string{"", "mdl 1;\n"} {
		file := writeScript(t, t.TempDir(), "s.test.mdl", header+tests)
		var code int
		out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{file}) })
		if strings.Contains(out, "module not found: MxTest") || code != 0 {
			t.Errorf("header %q: check rejected the runner's own module (exit %d):\n%s", header, code, out)
		}
	}

	// Control: a reference that does not resolve is still reported.
	bad := writeScript(t, t.TempDir(), "bad.test.mdl",
		"/** @test missing */\nRETRIEVE $x FROM NoSuchModule.NoSuchEntity;\n/\n")
	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{bad}) })
	if code == 0 || !strings.Contains(out, "NoSuchModule") {
		t.Errorf("a missing reference in a test body was not reported (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, "module not found: MxTest") {
		t.Errorf("the runner's module was reported alongside the real error:\n%s", out)
	}
}
