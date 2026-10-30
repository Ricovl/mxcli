// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/versions"
)

// ako/mxcli#1787: the workflow microflow actions Mendix added after 9.0 were
// refused only by the write (canon.UndeclaredTypeError), part-way through a
// script that `mxcli check -p` had passed. Each is now refused by check -p and
// by exec's pre-flight with MDL-VERSION01, naming the statement and the
// version, wherever it sits in the body. Each floor is its type's
// introduction in mendixmodelsdk 4.117.0:
//
//	call workflow, set task outcome, open user task, open workflow  9.0.2
//	workflow operation abort / restart                              9.2.0
//	workflow operation pause / continue                             9.3.0
//	get workflow data                                               9.10.0
//	workflow operation retry / unpause                              9.12.0
//	notify workflow                                                 10.0.0
//	get workflows, get workflow activity records                    10.4.0
func TestCheckVersionGates_WorkflowMicroflowActions(t *testing.T) {
	op := func(kind string) ast.MicroflowStatement {
		return &ast.WorkflowOperationStmt{OperationType: kind, WorkflowVariable: "Wf"}
	}
	for _, c := range []struct {
		stmt         ast.MicroflowStatement
		below, floor [3]int
		want         string
	}{
		{&ast.CallWorkflowStmt{Workflow: ast.QualifiedName{Module: "M", Name: "WF"}, ContextVariable: "Ctx"}, [3]int{9, 0, 1}, [3]int{9, 0, 2}, "call workflow"},
		{&ast.SetTaskOutcomeStmt{WorkflowTaskVariable: "Task", OutcomeValue: "Approve"}, [3]int{9, 0, 1}, [3]int{9, 0, 2}, "set task outcome"},
		{&ast.OpenUserTaskStmt{UserTaskVariable: "Task"}, [3]int{9, 0, 1}, [3]int{9, 0, 2}, "open user task"},
		{&ast.OpenWorkflowStmt{WorkflowVariable: "Wf"}, [3]int{9, 0, 1}, [3]int{9, 0, 2}, "open workflow"},
		{op("abort"), [3]int{9, 1, 0}, [3]int{9, 2, 0}, "workflow operation abort"},
		{op("restart"), [3]int{9, 1, 0}, [3]int{9, 2, 0}, "workflow operation restart"},
		{op("pause"), [3]int{9, 2, 0}, [3]int{9, 3, 0}, "workflow operation pause"},
		{op("continue"), [3]int{9, 2, 0}, [3]int{9, 3, 0}, "workflow operation continue"},
		{&ast.GetWorkflowDataStmt{OutputVariable: "Data", WorkflowVariable: "Wf", Workflow: ast.QualifiedName{Module: "M", Name: "WF"}}, [3]int{9, 9, 0}, [3]int{9, 10, 0}, "get workflow data"},
		{op("retry"), [3]int{9, 11, 0}, [3]int{9, 12, 0}, "workflow operation retry"},
		{op("unpause"), [3]int{9, 11, 0}, [3]int{9, 12, 0}, "workflow operation unpause"},
		{&ast.NotifyWorkflowStmt{WorkflowVariable: "Wf", Target: "M.WF.Wait"}, [3]int{9, 24, 30}, [3]int{10, 0, 0}, "notify workflow"},
		{&ast.GetWorkflowsStmt{OutputVariable: "Wfs", WorkflowContextVariableName: "Ctx"}, [3]int{10, 3, 0}, [3]int{10, 4, 0}, "get workflows"},
		{&ast.GetWorkflowActivityRecordsStmt{OutputVariable: "Records", WorkflowVariable: "Wf"}, [3]int{10, 3, 0}, [3]int{10, 4, 0}, "get workflow activity records"},
	} {
		floor := version.Version{Major: c.floor[0], Minor: c.floor[1], Patch: c.floor[2]}
		wantVersion := "requires Mendix " + floor.String() + "+"
		for _, place := range []struct {
			name string
			body []ast.MicroflowStatement
		}{
			{"top level", []ast.MicroflowStatement{c.stmt}},
			{"in a loop", []ast.MicroflowStatement{&ast.LoopStmt{LoopVariable: "It", ListVariable: "List", Body: []ast.MicroflowStatement{c.stmt}}}},
			{"in an else branch", []ast.MicroflowStatement{&ast.IfStmt{HasElse: true, ElseBody: []ast.MicroflowStatement{c.stmt}}}},
			{"in a while loop", []ast.MicroflowStatement{&ast.WhileStmt{Body: []ast.MicroflowStatement{c.stmt}}}},
			{"in an error handler", []ast.MicroflowStatement{&ast.RetrieveStmt{Variable: "X", Source: ast.QualifiedName{Module: "M", Name: "E"},
				ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustom, Body: []ast.MicroflowStatement{c.stmt}}}}},
		} {
			for _, stmt := range []ast.Statement{
				&ast.CreateMicroflowStmt{Name: ast.QualifiedName{Module: "M", Name: "MF"}, Body: place.body},
			} {
				prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, stmt}}
				vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
				if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want+" requires") ||
					!strings.Contains(vs[0].Message, wantVersion) || vs[0].Suggestion == "" {
					t.Errorf("%s %s (%T) below its floor %v: want one MDL-VERSION01 naming %q and %q with a hint, got %+v",
						c.want, place.name, stmt, c.below, c.want, wantVersion, vs)
				}
				// exec's handler refuses with the same text.
				if err := checkVersionGates(versionCtx(t, c.below[0], c.below[1], c.below[2]), stmt); err == nil ||
					len(vs) == 0 || !strings.Contains(vs[0].Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
					t.Errorf("%s %s (%T): exec's refusal %v differs from check's %+v", c.want, place.name, stmt, err, vs)
				}
				if vs := versionGateViolations(versionCtx(t, c.floor[0], c.floor[1], c.floor[2]), prog); len(vs) != 0 {
					t.Errorf("%s %s (%T): reported at its floor %v: %+v", c.want, place.name, stmt, c.floor, vs)
				}
			}
		}
	}
}

// The floor check -p applies to a type gate is the version the write refuses
// the type below, so the two cannot disagree: each gate's sdk/versions row
// opens exactly at canon.TypeIntroduced of the types the statement writes (the
// latest of them). A row typed with another version fails here, not as a
// script check passed and exec then refused part-way (ako/mxcli#1787). The
// #1774 workflow type gates are pinned with them.
func TestTypeGates_FloorIsTheTypeIntroduction(t *testing.T) {
	reg, err := versions.Load()
	if err != nil {
		t.Fatal(err)
	}
	pinned := append([]typeGate{
		{versionGate: waitForNotificationGate, types: []string{"Workflows$WaitForNotificationActivity"}},
		{versionGate: waitForTimerGate, types: []string{"Workflows$WaitForTimerActivity"}},
		{versionGate: timerBoundaryEventGate, types: []string{"Workflows$TimerBoundaryEvent"}},
		{versionGate: interruptingTimerBoundaryEventGate, types: []string{
			"Workflows$InterruptingTimerBoundaryEvent", "Workflows$NonInterruptingTimerBoundaryEvent"}},
		{versionGate: workflowDefinitionSelectionGate, types: []string{
			"Workflows$WorkflowDefinitionNameSelection", "Workflows$WorkflowDefinitionObjectSelection"}},
	}, workflowActionGates...)
	if len(workflowActionGates) == 0 {
		t.Fatal("no workflow action gates")
	}
	for _, g := range pinned {
		if len(g.types) == 0 {
			t.Errorf("%s: a type gate names no type", g.statement)
			continue
		}
		var floor version.Version
		for _, typ := range g.types {
			v, ok := canon.TypeIntroduced(typ)
			if !ok {
				t.Errorf("%s: %s has no introduction in the Model SDK data", g.statement, typ)
				continue
			}
			if floor.Compare(v) < 0 {
				floor = v
			}
		}
		at := versions.SemVer{Major: floor.Major, Minor: floor.Minor, Patch: floor.Patch}
		if !reg.IsAvailable(g.area, g.name, at) {
			t.Errorf("%s: %s.%s is not available at %s, where the write accepts %v", g.statement, g.area, g.name, at, g.types)
		}
		if below := justBelow(at); reg.IsAvailable(g.area, g.name, below) {
			t.Errorf("%s: %s.%s is available at %s, where the write refuses %v (introduced %s)", g.statement, g.area, g.name, below, g.types, at)
		}
	}
}

// justBelow is the last version before v.
func justBelow(v versions.SemVer) versions.SemVer {
	switch {
	case v.Patch > 0:
		v.Patch--
	case v.Minor > 0:
		v.Minor, v.Patch = v.Minor-1, 999
	default:
		v.Major, v.Minor, v.Patch = v.Major-1, 999, 999
	}
	return v
}

// Every workflow microflow statement either has a gate or is covered by a
// broader one. Lock and unlock (Microflows$Lock/UnlockWorkflowAction, 9.21) are
// the second: every lock or unlock check and exec accept names its workflow,
// and the selection that names it is 10.0 (workflowDefinitionSelectionGate).
func TestWorkflowActionGates_CoverEveryWorkflowMicroflowStatement(t *testing.T) {
	op := func(kind string) ast.MicroflowStatement { return &ast.WorkflowOperationStmt{OperationType: kind} }
	for _, st := range []ast.MicroflowStatement{
		&ast.CallWorkflowStmt{}, &ast.GetWorkflowDataStmt{}, &ast.GetWorkflowsStmt{}, &ast.GetWorkflowActivityRecordsStmt{},
		op("abort"), op("continue"), op("pause"), op("restart"), op("retry"), op("unpause"),
		&ast.SetTaskOutcomeStmt{}, &ast.OpenUserTaskStmt{}, &ast.NotifyWorkflowStmt{}, &ast.OpenWorkflowStmt{},
	} {
		n := 0
		for _, g := range workflowActionGates {
			if g.match(st) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%T %+v: matched by %d workflow action gates, want 1", st, st, n)
		}
	}
	for _, st := range []ast.MicroflowStatement{&ast.LockWorkflowStmt{Workflow: "M.WF"}, &ast.UnlockWorkflowStmt{WorkflowVariable: "Def"}} {
		if !selectsWorkflowDefinition(st) {
			t.Errorf("%T naming a workflow is not gated by the 10.0 selection", st)
		}
		for _, g := range workflowActionGates {
			if g.match(st) {
				t.Errorf("%T matched by %q as well as by the selection gate", st, g.statement)
			}
		}
	}
}
