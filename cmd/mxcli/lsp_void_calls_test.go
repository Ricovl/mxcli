// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"go.lsp.dev/uri"
)

func lspRules(t *testing.T, s *mdlServer, text string) string {
	t.Helper()
	var rules []string
	for _, d := range s.documentDiagnostics(uri.File("/w/s.mdl"), text) {
		rules = append(rules, fmt.Sprintf("[%v] %s", d.Code, d.Message))
	}
	return strings.Join(rules, "\n")
}

// ako/mxcli#962 item 3: the editor validated flows without the project, so two
// calls to a stored VOID JavaScript action (the Studio Pro shape; mxbuild
// 11.13.0 builds it clean, #953) were squiggled MDL063. With the workspace's
// project it resolves the action; without one, an action it cannot resolve is
// treated as possibly void. Controls: a Boolean action's pair is still
// reported, and a read of a known-void call's output is MDL093.
func TestLSP_VoidCallsResolveThroughTheProject(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	withProject := &mdlServer{mprPath: filepath.Join(dir, "PedApp.mpr"), codeActions: executor.NewCodeActionCache()}
	noProject := &mdlServer{}

	voidPair := `create nanoflow MyFirstModule.NF_RevokeTwice ()
begin
  $ReturnValueName = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = 'a');
  $ReturnValueName = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = 'b');
end;
`
	if got := lspRules(t, withProject, voidPair); strings.Contains(got, "MDL063") {
		t.Errorf("with the project: two calls to a stored void action squiggled:\n%s", got)
	}
	if got := lspRules(t, noProject, voidPair); strings.Contains(got, "MDL063") {
		t.Errorf("without a project: an unresolvable pair squiggled, not treated as possibly void:\n%s", got)
	}

	boolPair := `create nanoflow MyFirstModule.NF_StrictTwice ()
begin
  $IsStrict = call javascript action FeedbackModule.JS_isStrictMode();
  $IsStrict = call javascript action FeedbackModule.JS_isStrictMode();
end;
`
	if got := lspRules(t, withProject, boolPair); !strings.Contains(got, "MDL063") {
		t.Errorf("control: two calls to a stored Boolean action not reported:\n%s", got)
	}

	readVoid := `create nanoflow MyFirstModule.NF_ReadVoid ()
begin
  $V = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = 'a');
  $S = call javascript action FeedbackModule.JS_RevokeUploadedFileFromMemory(fileBlobURL = $V);
end;
`
	if got := lspRules(t, withProject, readVoid); !strings.Contains(got, "MDL093") {
		t.Errorf("with the project: a read of a void call's output not reported:\n%s", got)
	}
	if got := lspRules(t, noProject, readVoid); strings.Contains(got, "MDL093") {
		t.Errorf("without a project: a possibly-void call's output read reported as MDL093:\n%s", got)
	}
}
