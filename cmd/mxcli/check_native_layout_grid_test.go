// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#962 item 4, end to end on PedApp: `check -p` reads from the
// project that Atlas_Core.NativePhone_Default is a native layout and does not
// give a page on it the layout-grid advice (MPR010) — on mxbuild 11.13.0 the
// bare form builds clean there and a layoutgrid is CE6858. The web page in the
// same script is the control.
func TestCheck_MPR010SkipsNativePages(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	_ = checkCmd.InheritedFlags() // merges the persistent -p into checkCmd.Flags()
	_ = rootCmd.PersistentFlags().Set("project", filepath.Join(dir, "PedApp.mpr"))
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	file := writeScript(t, t.TempDir(), "s.mdl", `mdl 1;
create persistent entity MyFirstModule.Thing (Name: String(100));
create page MyFirstModule.P_NativeForm (Title: 'N', Layout: Atlas_Core.NativePhone_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvNative (DataSource: $Thing) { textbox tb1 (Attribute: Name) }
};
create page MyFirstModule.P_WebForm (Title: 'W', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvWeb (DataSource: $Thing) { textbox tb2 (Attribute: Name) }
};
`)
	out := captureStd(t, func() { runCheckFiles(checkCmd, []string{file}) })
	if strings.Contains(out, "dvNative") {
		t.Errorf("MPR010 advised a layoutgrid on a native page:\n%s", out)
	}
	if !strings.Contains(out, "dvWeb") || !strings.Contains(out, "MPR010") {
		t.Errorf("control: the web page's bare form not reported:\n%s", out)
	}
}
