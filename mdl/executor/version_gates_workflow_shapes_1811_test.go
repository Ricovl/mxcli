// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// statementGateCase is one statement a document or workflow shape gate names.
type statementGateCase struct {
	name         string
	stmt         ast.Statement
	below, floor [3]int
	want         string
}

// checkStatementGateCases asserts that check -p reports the statement below
// the floor with MDL-VERSION01 naming it and the version, that exec's handler
// refuses with the same text, and that the statement is not reported for it
// at its floor.
func checkStatementGateCases(t *testing.T, cases []statementGateCase) {
	t.Helper()
	for _, c := range cases {
		floor := version.Version{Major: c.floor[0], Minor: c.floor[1], Patch: c.floor[2]}
		wantVersion := "requires Mendix " + floor.String() + "+"
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, c.stmt}}
		vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
		if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want+" requires") ||
			!strings.Contains(vs[0].Message, wantVersion) || vs[0].Suggestion == "" {
			t.Errorf("%s below its floor %v: want one MDL-VERSION01 naming %q and %q with a hint, got %+v",
				c.name, c.below, c.want, wantVersion, vs)
		}
		if err := checkVersionGates(versionCtx(t, c.below[0], c.below[1], c.below[2]), c.stmt); err == nil ||
			len(vs) == 0 || !strings.Contains(vs[0].Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
			t.Errorf("%s: exec's refusal %v differs from check's %+v", c.name, err, vs)
		}
		for _, v := range versionGateViolations(versionCtx(t, c.floor[0], c.floor[1], c.floor[2]), prog) {
			if strings.Contains(v.Message, c.want+" requires") {
				t.Errorf("%s: reported at its floor %v: %+v", c.name, c.floor, v)
			}
		}
	}
}

// ako/mxcli#1811 sweep, the workflow shapes: a workflow's own floor is 9.0.2
// (#1809); what it states can be later. Measured on stamped 9.x projects: a parameter is refused below
// 9.18 and a user task below 9.19.
func TestCheckVersionGates_WorkflowShapes1811(t *testing.T) {
	wf := ast.QualifiedName{Module: "M", Name: "WF"}
	call := &ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: "Act"}}
	create := func(acts ...ast.WorkflowActivityNode) *ast.CreateWorkflowStmt {
		return &ast.CreateWorkflowStmt{Name: wf, Activities: acts}
	}
	withParam := create(call)
	withParam.ParameterVar, withParam.ParameterEntity = "$WorkflowContext", ast.QualifiedName{Module: "M", Name: "Request"}
	withOverview := create(call)
	withOverview.OverviewPage = ast.QualifiedName{Module: "M", Name: "Overview"}
	withHeaderNote := create(call)
	withHeaderNote.Annotation = "note"
	annotated := &ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: "Act"}}
	annotated.Annotation = "note"
	userTask := &ast.WorkflowUserTaskNode{Name: "Review", Outcomes: []ast.WorkflowUserTaskOutcomeNode{{Caption: "Done"}}}
	callWf := &ast.WorkflowCallWorkflowNode{Workflow: ast.QualifiedName{Module: "M", Name: "Sub"}}
	callWfMapped := &ast.WorkflowCallWorkflowNode{Workflow: ast.QualifiedName{Module: "M", Name: "Sub"},
		ParameterMappings: []ast.WorkflowParameterMappingNode{{Parameter: "WorkflowContext", Expression: "$WorkflowContext"}}}
	esp := create(call)
	esp.EventSubProcesses = []ast.WorkflowEventSubProcessNode{{Name: "Cancel", Interrupting: true, Activities: []ast.WorkflowActivityNode{call}}}
	insert := func(act ast.WorkflowActivityNode) *ast.AlterWorkflowStmt {
		return &ast.AlterWorkflowStmt{Name: wf, Operations: []ast.AlterWorkflowOp{&ast.InsertAfterOp{NewActivities: []ast.WorkflowActivityNode{act}}}}
	}
	checkStatementGateCases(t, []statementGateCase{
		{"a jump", create(call, &ast.WorkflowJumpToNode{Target: "act"}), [3]int{9, 0, 4}, [3]int{9, 0, 5}, "jump to"},
		{"a call workflow activity", create(callWf), [3]int{9, 5, 0}, [3]int{9, 6, 0}, "call workflow (an activity)"},
		{"an inserted call workflow activity", insert(callWf), [3]int{9, 5, 0}, [3]int{9, 6, 0}, "call workflow (an activity)"},
		{"an overview page", withOverview, [3]int{9, 10, 0}, [3]int{9, 11, 0}, "overview page (of a workflow)"},
		{"alter workflow set the overview page", &ast.AlterWorkflowStmt{Name: wf, Operations: []ast.AlterWorkflowOp{
			&ast.SetWorkflowPropertyOp{Property: "OVERVIEW_PAGE", Entity: ast.QualifiedName{Module: "M", Name: "Overview"}}}},
			[3]int{9, 10, 0}, [3]int{9, 11, 0}, "overview page (of a workflow)"},
		{"the workflow's annotation", withHeaderNote, [3]int{9, 14, 0}, [3]int{9, 15, 0}, "an annotation in a workflow"},
		{"an activity's annotation", create(annotated), [3]int{9, 14, 0}, [3]int{9, 15, 0}, "an annotation in a workflow"},
		{"a parameter", withParam, [3]int{9, 17, 0}, [3]int{9, 18, 0}, "parameter $WorkflowContext: …"},
		{"alter workflow set the parameter", &ast.AlterWorkflowStmt{Name: wf, Operations: []ast.AlterWorkflowOp{
			&ast.SetWorkflowPropertyOp{Property: "PARAMETER", Entity: ast.QualifiedName{Module: "M", Name: "Request"}}}},
			[3]int{9, 17, 0}, [3]int{9, 18, 0}, "parameter $WorkflowContext: …"},
		{"call workflow with mappings", create(callWfMapped), [3]int{9, 17, 0}, [3]int{9, 18, 0}, "call workflow … ( mappings ) (an activity)"},
		{"a user task", create(userTask), [3]int{9, 18, 0}, [3]int{9, 19, 0}, "user task"},
		{"an inserted user task", insert(userTask), [3]int{9, 18, 0}, [3]int{9, 19, 0}, "user task"},
		{"an interrupting notification event sub-process", esp, [3]int{11, 9, 0}, [3]int{11, 10, 0},
			"event subprocess … on interrupting notification"},
	})
}
