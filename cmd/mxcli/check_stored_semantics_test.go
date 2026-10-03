// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#943: two semantic rules answer differently once the stored model
// is read, and `check -p` did not read it where `exec -p` did.
//
//   - MDL067 (a bare commit now means WITH events) is noise for a flow already
//     stored that way; exec dropped it, check printed it. Control: a flow
//     stored WITHOUT events, which the bare commit flips, is still noted.
//   - MDL-WORKFLOW10 ignored a claim made in a called microflow. A stored
//     callee that claims silences it; control: one that does not still warns.
func TestCheckProject_ReadsStoredFlowsForSemanticRules(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")

	const setup = `create or modify microflow MyFirstModule.Settled ($U: System.User)
begin
  commit $U;
end;
create or modify microflow MyFirstModule.Flipped ($U: System.User)
begin
  commit $U without events;
end;
create or modify microflow MyFirstModule.SUB_Claim ($T: System.WorkflowUserTask)
begin
  change $T (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
end;
create or modify microflow MyFirstModule.SUB_NoClaim ($T: System.WorkflowUserTask)
begin
  log info 'nothing';
end;
`
	exe := executor.New(io.Discard)
	exe.SetBackendFactory(newBackendFactory())
	prog, errs := visitor.Build(fmt.Sprintf("connect local '%s';\n%s", visitor.QuoteString(mpr), setup))
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if err := exe.ExecuteProgram(prog); err != nil {
		t.Fatalf("store the flows: %v", err)
	}
	_ = exe.Close()

	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()

	check := func(script string) string {
		file := writeScript(t, t.TempDir(), "s.mdl", "mdl 1;\n"+script)
		var code int
		out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{file}) })
		if code != 0 {
			t.Fatalf("check failed (exit %d):\n%s", code, out)
		}
		return out
	}

	settled := check("create or modify microflow MyFirstModule.Settled ($U: System.User)\nbegin\n  commit $U;\nend;\n")
	if strings.Contains(settled, "MDL067") {
		t.Errorf("MDL067 printed for a commit stored exactly as the script writes it:\n%s", settled)
	}
	flipped := check("create or modify microflow MyFirstModule.Flipped ($U: System.User)\nbegin\n  commit $U;\nend;\n")
	if !strings.Contains(flipped, "MDL067") {
		t.Errorf("control: MDL067 missing for a bare commit that flips the stored events:\n%s", flipped)
	}

	claimed := check("create or modify microflow MyFirstModule.ACT_A ($Task: System.WorkflowUserTask)\nbegin\n" +
		"  call microflow MyFirstModule.SUB_Claim(T = $Task);\n  set task outcome $Task 'Plan';\nend;\n")
	if strings.Contains(claimed, "MDL-WORKFLOW10") {
		t.Errorf("MDL-WORKFLOW10 for a task claimed by the stored callee:\n%s", claimed)
	}
	unclaimed := check("create or modify microflow MyFirstModule.ACT_B ($Task: System.WorkflowUserTask)\nbegin\n" +
		"  call microflow MyFirstModule.SUB_NoClaim(T = $Task);\n  set task outcome $Task 'Plan';\nend;\n")
	if !strings.Contains(unclaimed, "MDL-WORKFLOW10") {
		t.Errorf("control: no MDL-WORKFLOW10 for a stored callee that does not claim:\n%s", unclaimed)
	}
}
