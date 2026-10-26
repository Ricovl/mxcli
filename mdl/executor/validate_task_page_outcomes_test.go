// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A user task whose page offers no way to choose one of its outcomes passed
// check, mx check and the build, and every instance stalled at the task: an
// agent eval generated three such pages (ako/mxcli#1709, #1710).

const taskPageWorkflow = `create workflow M.W
  parameter $Context: M.Request
begin
  user task Review 'Review' page M.WF_Review outcomes 'Approve' { } 'Reject' { };
end workflow;
`

func taskPageProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog
}

func taskPage(widgets string) string {
	return `create page M.WF_Review
(
  params: ( $WorkflowUserTask: System.WorkflowUserTask ),
  title: 'Review',
  layout: Atlas_Core.Atlas_Default
)
{
  dataview dvTask (DataSource: $WorkflowUserTask) {
` + widgets + `
  }
};
`
}

func wf20(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == taskPageOutcomeRule {
			out = append(out, v)
		}
	}
	return out
}

func TestTaskPageOutcomes_ReportsAnOutcomeThePageCannotChoose(t *testing.T) {
	for _, tc := range []struct {
		name, widgets string
		missing       []string
	}{
		{"a page with no outcome buttons", `    dynamictext txt (Content: 'Review')`, []string{"'Approve'", "'Reject'"}},
		{"a page with one of two", `    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')`, []string{"'Reject'"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs := wf20(ValidateTaskPageOutcomes(taskPageProgram(t, taskPage(tc.widgets)+taskPageWorkflow)))
			if len(vs) != 1 {
				t.Fatalf("got %d MDL-WF20 warnings, want 1: %v", len(vs), vs)
			}
			if vs[0].Severity != linter.SeverityWarning {
				t.Errorf("severity = %v, want a warning (an outcome may be completed off the page on purpose)", vs[0].Severity)
			}
			for _, w := range append([]string{"Review", "M.WF_Review", "complete task"}, tc.missing...) {
				if !strings.Contains(vs[0].Message+vs[0].Suggestion, w) {
					t.Errorf("warning should mention %s: %s / %s", w, vs[0].Message, vs[0].Suggestion)
				}
			}
		})
	}
}

// CONTROL: every page here lets the user choose each outcome.
func TestTaskPageOutcomes_SilentWhenEveryOutcomeIsReachable(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a complete task button per outcome", taskPage(`
    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')
    actionbutton btnReject (Caption: 'Reject', Action: complete task 'Reject')`)},
		{"a microflow that sets the other outcome", taskPage(`
    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')
    actionbutton btnReject (Caption: 'Reject', Action: call microflow M.ACT_Reject(Task = $WorkflowUserTask))`) + `
create microflow M.ACT_Reject ($Task: System.WorkflowUserTask)
begin
  change $Task (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $Task;
  set task outcome $Task 'Reject';
end;
`},
		{"through a second microflow", taskPage(`
    actionbutton btnDecide (Caption: 'Decide', Action: call microflow M.ACT_Decide(Task = $WorkflowUserTask))`) + `
create microflow M.ACT_Decide ($Task: System.WorkflowUserTask)
begin
  call microflow M.ACT_Complete(Task = $Task);
end;
create microflow M.ACT_Complete ($Task: System.WorkflowUserTask)
begin
  if $Task/Name = 'x' then
    set task outcome $Task 'Approve';
  else
    set task outcome $Task 'Reject';
  end if;
end;
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if vs := wf20(ValidateTaskPageOutcomes(taskPageProgram(t, tc.src+taskPageWorkflow))); len(vs) != 0 {
				t.Errorf("reported, but every outcome is reachable: %v", vs)
			}
		})
	}
}

// What the script cannot show is not a finding: a button calling a microflow
// that is not in the script, a snippet the page embeds, a page the script does
// not create, and a page the script alters.
func TestTaskPageOutcomes_SilentWhenThePageCannotBeJudged(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a microflow the script does not create", taskPage(`
    actionbutton btnAll (Caption: 'Decide', Action: call microflow M.ACT_Elsewhere(Task = $WorkflowUserTask))`)},
		{"a snippet", taskPage(`
    snippetcall scButtons (Snippet: M.TaskButtons)`)},
		{"a page not in the script", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if vs := wf20(ValidateTaskPageOutcomes(taskPageProgram(t, tc.src+taskPageWorkflow))); len(vs) != 0 {
				t.Errorf("reported a page the script cannot show: %v", vs)
			}
		})
	}
}

// A multi-user task is completed through the same page actions.
func TestTaskPageOutcomes_CoversMultiUserTasks(t *testing.T) {
	src := taskPage(`    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')`) + `create workflow M.W
  parameter $Context: M.Request
begin
  multi user task Board 'Board' page M.WF_Review outcomes 'Approve' { } 'Reject' { };
end workflow;
`
	if vs := wf20(ValidateTaskPageOutcomes(taskPageProgram(t, src))); len(vs) != 1 {
		t.Fatalf("got %d warnings, want 1 for the multi-user task: %v", len(vs), vs)
	}
}

// storedTaskPage is a stored page document with the given actions, shaped like
// the one mxbuild 11.15.0's project held for the skill's template page.
func storedTaskPage(t *testing.T, actions ...bson.D) []byte {
	t.Helper()
	var widgets bson.A
	widgets = append(widgets, int32(2))
	for i, a := range actions {
		widgets = append(widgets, bson.D{{Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: "b" + string(rune('a'+i))}, {Key: "Action", Value: a}})
	}
	doc := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{{Key: "Arguments", Value: bson.A{int32(2), bson.D{{Key: "Widgets", Value: widgets}}}}}},
	}
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func completeTaskAction(outcome string) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$SetTaskOutcomeClientAction"}, {Key: "OutcomeValue", Value: outcome}}
}

// With a project, a page the script does not create is read from it.
func TestStoredTaskPageOutcomes_ReadsAStoredPage(t *testing.T) {
	prog := taskPageProgram(t, taskPageWorkflow)
	for _, tc := range []struct {
		name    string
		page    []byte
		wantOne bool
	}{
		{"one of two outcomes", storedTaskPage(t, completeTaskAction("Approve")), true},
		{"both outcomes", storedTaskPage(t, completeTaskAction("Approve"), completeTaskAction("Reject")), false},
		{"a snippet", storedTaskPage(t, completeTaskAction("Approve"), bson.D{{Key: "$Type", Value: "Forms$SnippetCallWidget"}}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb := &mock.MockBackend{
				GetRawUnitByNameFunc: func(kind, qn string) (*types.RawUnitInfo, error) {
					if kind == "page" && qn == "M.WF_Review" {
						return &types.RawUnitInfo{QualifiedName: qn, Contents: tc.page}, nil
					}
					return nil, nil
				},
			}
			vs := wf20(StoredTaskPageOutcomeViolations(prog, mb))
			if got := len(vs) == 1; got != tc.wantOne || len(vs) > 1 {
				t.Errorf("got %v, want one warning = %v", vs, tc.wantOne)
			}
		})
	}
}

// The stored pass adds only what the script-only pass did not report, so check
// -p, which prints both, says it once.
func TestStoredTaskPageOutcomes_DoesNotRepeatTheScriptPass(t *testing.T) {
	prog := taskPageProgram(t, taskPage(`    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve')`)+taskPageWorkflow)
	if n := len(wf20(ValidateTaskPageOutcomes(prog))); n != 1 {
		t.Fatalf("control: the script pass reported %d, want 1", n)
	}
	if vs := wf20(StoredTaskPageOutcomeViolations(prog, &mock.MockBackend{})); len(vs) != 0 {
		t.Errorf("the stored pass repeated the script pass's warning: %v", vs)
	}
}
