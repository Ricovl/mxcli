// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#870. A Lock workflow activity always names a workflow
// definition; PauseAllWorkflows is Studio Pro's "Pause instances" checkbox
// (Unlock's is "Unpause instances", ResumeAllPausedWorkflows), on by default.
// WorkflowCommons.ACT_WorkflowDefinition_Lock in ako/TestApp stores both
// branches of that choice: PauseAllWorkflows true WITH a WorkflowDefinition
// variable selection, and false with the same selection.
//
// MDL read the flag as "all workflows": `lock workflow all;` wrote the flag and
// no selection, which mxbuild rejects with CE1825 "The 'Workflow' property is
// required", and DESCRIBE printed the Studio Pro activity as that same
// `lock workflow all;` — dropping the workflow, so describe → exec turned a
// working microflow into one that does not build.

func lockStmt(t *testing.T, src string) ast.MicroflowStatement {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\ncreate microflow M.MF ($Wf: System.WorkflowDefinition)\nbegin\n" + src + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("%s: %v", src, errs)
	}
	return prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt).Body[0]
}

func TestLockWorkflow_PauseInstancesIsAFlagOnASelection(t *testing.T) {
	for _, tc := range []struct {
		src            string
		variable, name string
		flag           bool
	}{
		{"lock workflow $Wf;", "Wf", "", false},
		{"lock workflow $Wf pause all;", "Wf", "", true},
		{"lock workflow M.Approve pause all;", "", "M.Approve", true},
		{"unlock workflow $Wf;", "Wf", "", false},
		{"unlock workflow $Wf unpause all;", "Wf", "", true},
		{"unlock workflow M.Approve;", "", "M.Approve", false},
	} {
		fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
		oc := fb.buildFlowGraph([]ast.MicroflowStatement{lockStmt(t, tc.src)}, nil)
		var variable, name string
		var flag, found bool
		for _, o := range oc.Objects {
			aa, ok := o.(*microflows.ActionActivity)
			if !ok {
				continue
			}
			switch a := aa.Action.(type) {
			case *microflows.LockWorkflowAction:
				variable, name, flag, found = a.WorkflowVariable, a.Workflow, a.PauseAllWorkflows, true
			case *microflows.UnlockWorkflowAction:
				variable, name, flag, found = a.WorkflowVariable, a.Workflow, a.ResumeAllPausedWorkflows, true
			}
		}
		if !found {
			t.Errorf("%s: no action built", tc.src)
			continue
		}
		if variable != tc.variable || name != tc.name || flag != tc.flag {
			t.Errorf("%s: built var=%q name=%q flag=%v, want var=%q name=%q flag=%v",
				tc.src, variable, name, flag, tc.variable, tc.name, tc.flag)
		}
	}
}

// The Studio Pro activity describes as a statement that rebuilds it.
func TestLockWorkflow_DescribeKeepsTheSelection(t *testing.T) {
	for _, tc := range []struct {
		action microflows.MicroflowAction
		want   string
	}{
		{&microflows.LockWorkflowAction{PauseAllWorkflows: true, WorkflowVariable: "WorkflowDefinition"}, "lock workflow $WorkflowDefinition pause all;"},
		{&microflows.LockWorkflowAction{WorkflowVariable: "WorkflowDefinition"}, "lock workflow $WorkflowDefinition;"},
		{&microflows.LockWorkflowAction{PauseAllWorkflows: true, Workflow: "M.Approve"}, "lock workflow M.Approve pause all;"},
		{&microflows.UnlockWorkflowAction{ResumeAllPausedWorkflows: true, WorkflowVariable: "WorkflowDefinition"}, "unlock workflow $WorkflowDefinition unpause all;"},
		{&microflows.UnlockWorkflowAction{Workflow: "M.Approve"}, "unlock workflow M.Approve;"},
	} {
		if got := formatAction(&ExecContext{}, tc.action, nil, nil); got != tc.want {
			t.Errorf("describe: got %q, want %q", got, tc.want)
		}
		// ... and the description parses back to the same activity.
		lockStmt(t, tc.want)
	}
}

// `lock workflow all` names no workflow, and there is no model for it: refused
// at check and exec rather than written into a microflow that cannot build.
func TestLockWorkflow_AllWithoutAWorkflowIsRefused(t *testing.T) {
	for _, src := range []string{"lock workflow all;", "unlock workflow all;"} {
		prog, errs := visitor.Build("create microflow M.MF ()\nbegin\n" + src + "\nend;")
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		mf := prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)
		if vs := ValidateMicroflow(mf); !hasErrorRule(vs, lockWorkflowAllRule) {
			t.Errorf("%s: check passed it: %+v", src, vs)
		}
		if err := validateMicroflowRules(mf); err == nil {
			t.Errorf("%s: exec accepted it", src)
		}
	}
	// CONTROL: a lock that names its workflow is clean.
	prog, _ := visitor.Build("create microflow M.MF ($Wf: System.WorkflowDefinition)\nbegin\nlock workflow $Wf pause all;\nunlock workflow $Wf;\nend;")
	if vs := ValidateMicroflow(prog.Statements[len(prog.Statements)-1].(*ast.CreateMicroflowStmt)); hasErrorRule(vs, lockWorkflowAllRule) {
		t.Errorf("a lock naming its workflow was refused: %+v", vs)
	}
}
