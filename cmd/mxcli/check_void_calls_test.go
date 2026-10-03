// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#953 against a real project (PedApp's FeedbackModule, a Marketplace
// module Studio Pro authored):
//
//   - item 1: a call to a stored VOID JavaScript action declares no variable,
//     so two calls carrying the same output name are not MDL063 (mxbuild
//     11.13.0 builds them clean). Control: the same pair on a Boolean action is.
//   - item 7: `check --references` stopped at the first tier with an error, so
//     a semantic error hid a call naming a parameter the Java action does not
//     have (CE1613). The reference tier now runs and reports it too.
func TestCheckReferences_VoidCallsAndHiddenParameterErrors(t *testing.T) {
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
	_ = checkCmd.Flags().Set("references", "true")
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
		_ = checkCmd.Flags().Set("references", "false")
		checkCmd.Flags().Lookup("references").Changed = false
	}()

	check := func(script string) (int, string) {
		file := writeScript(t, t.TempDir(), "s.mdl", "mdl 1;\n"+script)
		var code int
		out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{file}) })
		return code, out
	}

	code, out := check(`create nanoflow MyFirstModule.NF_RevokeTwice ()
begin
  $ReturnValueName = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = 'a');
  $ReturnValueName = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = 'b');
end;
`)
	if code != 0 || strings.Contains(out, "MDL063") || strings.Contains(out, "already declared") {
		t.Errorf("two calls to a void action reported as a duplicate (exit %d):\n%s", code, out)
	}

	code, out = check(`create nanoflow MyFirstModule.NF_StrictTwice ()
begin
  $IsStrict = call javascript action FeedbackModule.JS_isStrictMode();
  $IsStrict = call javascript action FeedbackModule.JS_isStrictMode();
end;
`)
	if code == 0 || !strings.Contains(out, "MDL063") {
		t.Errorf("control: two calls to a Boolean action not reported (exit %d):\n%s", code, out)
	}

	code, out = check(`create microflow MyFirstModule.MF_SaveLog ($Email: String)
begin
  if $Email != empty then
    $Valid = call java action FeedbackModule.ValidateEmail(email = $Email);
  else
    $Valid = call java action FeedbackModule.ValidateEmail(email = 'x@y.z');
  end if;
end;
`)
	if code == 0 || !strings.Contains(out, "MDL063") {
		t.Errorf("the real duplicate across branches is not reported (exit %d):\n%s", code, out)
	}
	if !strings.Contains(out, `has no parameter "email"`) {
		t.Errorf("the unknown parameter (CE1613) is hidden behind the semantic error:\n%s", out)
	}
}
