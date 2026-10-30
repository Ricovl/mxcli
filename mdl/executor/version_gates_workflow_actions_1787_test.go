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
// version, wherever it sits in the body, in `create microflow` and in what
// `alter microflow` inserts. Each floor is its type's introduction in
// mendixmodelsdk 4.117.0:
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
				&ast.AlterFlowStmt{Name: ast.QualifiedName{Module: "M", Name: "MF"}, Operations: []*ast.AlterFlowOperation{
					{Op: ast.AlterFlowInsertAfter, Target: "$X", Body: place.body}}},
				&ast.AlterFlowStmt{Name: ast.QualifiedName{Module: "M", Name: "MF"}, Operations: []*ast.AlterFlowOperation{
					{Op: ast.AlterFlowDrop, Target: "$Y"},
					{Op: ast.AlterFlowReplace, Target: "$X", Body: place.body}}},
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
				// At its floor the action is not reported. A loop placement
				// below 9.0.4 is — for the loop, gated itself since
				// ako/mxcli#1811 — and nothing else.
				for _, v := range versionGateViolations(versionCtx(t, c.floor[0], c.floor[1], c.floor[2]), prog) {
					loop := strings.Contains(place.name, "loop") && c.floor[0] == 9 && c.floor[1] == 0 && c.floor[2] < 4
					if !loop || !strings.Contains(v.Message, "loop … in $List requires") && !strings.Contains(v.Message, "while requires") {
						t.Errorf("%s %s (%T): reported at its floor %v: %+v", c.want, place.name, stmt, c.floor, v)
					}
				}
			}
		}
	}
}

// An `alter microflow` is gated on what it inserts, and on nothing else: a
// drop, or a fragment with no gated action, passes on 9.0.0 — and an alter
// nanoflow that inserts `synchronize unsynchronized` is gated as create
// nanoflow is.
func TestCheckVersionGates_AlterFlowGatesWhatItInserts(t *testing.T) {
	plain := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
		&ast.AlterFlowStmt{Operations: []*ast.AlterFlowOperation{
			{Op: ast.AlterFlowDrop, Target: "$X"},
			{Op: ast.AlterFlowInsertBefore, Target: "$Y", Body: []ast.MicroflowStatement{&ast.LogStmt{}}},
		}}}}
	if vs := versionGateViolations(versionCtx(t, 9, 0, 0), plain); len(vs) != 0 {
		t.Errorf("an alter microflow inserting no gated action reported on 9.0.0: %+v", vs)
	}
	unsync := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
		&ast.AlterFlowStmt{Nanoflow: true, Operations: []*ast.AlterFlowOperation{
			{Op: ast.AlterFlowReplaceBody, Target: "loop $It in $List", Body: []ast.MicroflowStatement{
				&ast.SynchronizeStmt{SyncType: "Unsynchronized"}}}}}}}
	if vs := versionGateViolations(versionCtx(t, 9, 3, 0), unsync); len(vs) != 1 || !strings.Contains(vs[0].Message, "synchronize unsynchronized") {
		t.Errorf("alter nanoflow inserting synchronize unsynchronized on 9.3: %+v", vs)
	}
	if vs := versionGateViolations(versionCtx(t, 9, 4, 0), unsync); len(vs) != 0 {
		t.Errorf("alter nanoflow inserting synchronize unsynchronized on 9.4: %+v", vs)
	}
	// The other microflow body gates reach an alter too: send email (11.13)
	// and a lock naming its workflow (10.0).
	for _, c := range []struct {
		stmt  ast.MicroflowStatement
		below [3]int
		want  string
	}{
		{&ast.SendEmailStmt{}, [3]int{11, 12, 0}, "send email"},
		{&ast.LockWorkflowStmt{Workflow: "M.WF"}, [3]int{9, 24, 30}, "lock workflow / unlock workflow on a workflow"},
	} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
			&ast.AlterFlowStmt{Operations: []*ast.AlterFlowOperation{{Op: ast.AlterFlowInsertAfter, Target: "$X",
				Body: []ast.MicroflowStatement{c.stmt}}}}}}
		if vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog); len(vs) != 1 || !strings.Contains(vs[0].Message, c.want) {
			t.Errorf("alter microflow inserting %T on %v: %+v", c.stmt, c.below, vs)
		}
	}
}

// exec reaches `alter microflow` without the pre-flight too (the REPL,
// --no-check): its handler refuses the gated insert before it reads, let
// alone writes, the stored flow — the mock backend's microflow calls are not
// configured, so reaching one would fail with another error.
func TestExecAlterFlow_RefusesAGatedInsertBeforeReading(t *testing.T) {
	stmt := &ast.AlterFlowStmt{Name: ast.QualifiedName{Module: "M", Name: "MF"}, Operations: []*ast.AlterFlowOperation{
		{Op: ast.AlterFlowInsertAfter, Target: "$X", Body: []ast.MicroflowStatement{
			&ast.GetWorkflowsStmt{OutputVariable: "Wfs", WorkflowContextVariableName: "Ctx"}}}}}
	err := execAlterFlow(versionCtx(t, 9, 24, 30), stmt)
	if err == nil || !strings.Contains(err.Error(), "get workflows requires Mendix 10.4.0+ (project is") {
		t.Fatalf("alter microflow inserting get workflows on 9.24.30: %v", err)
	}
}

// The floor check -p applies to a type gate is the version the write refuses
// the type below, so the two cannot disagree: each gate's sdk/versions row
// opens exactly at canon.TypeIntroduced of the types the statement writes, or
// at the version canon.PropertyDeclared first declares a property it writes a
// value to, whichever is latest (ako/mxcli#1811: a 9.0.2 type can sit in a
// 9.19 property). A row typed with another version fails here, not as a
// script check passed and exec then refused part-way (ako/mxcli#1787). The
// #1774 workflow type gates are pinned with them.
func TestTypeGates_FloorIsTheTypeIntroduction(t *testing.T) {
	reg, err := versions.Load()
	if err != nil {
		t.Fatal(err)
	}
	pinned := allTypeGates()
	if len(workflowActionGates) == 0 || len(flowActionGates) == 0 || len(workflowShapeGates) == 0 || len(documentTypeGates) == 0 {
		t.Fatal("no workflow action, flow action, workflow shape or document gates")
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
		for _, prop := range g.properties {
			typ, key, _ := strings.Cut(prop, ".")
			v, ok := propertyIntroduction(typ, key)
			if !ok {
				t.Errorf("%s: %s is declared by no version the data knows", g.statement, prop)
				continue
			}
			if floor.Compare(v) < 0 {
				floor = v
			}
		}
		at := versions.SemVer{Major: floor.Major, Minor: floor.Minor, Patch: floor.Patch}
		if !reg.IsAvailable(g.area, g.name, at) {
			t.Errorf("%s: %s.%s is not available at %s, where the write accepts %v %v", g.statement, g.area, g.name, at, g.types, g.properties)
		}
		if below := justBelow(at); reg.IsAvailable(g.area, g.name, below) {
			t.Errorf("%s: %s.%s is available at %s, where the write refuses %v %v (introduced %s)", g.statement, g.area, g.name, below, g.types, g.properties, at)
		}
	}
}

// propertyIntroduction is the first version canon.PropertyDeclared declares
// typ.key in: the floor the write refuses a value of it below.
func propertyIntroduction(typ, key string) (version.Version, bool) {
	if canon.PropertyDeclared(typ, key, version.Version{Major: 7}) {
		return version.Version{}, false // no data: declared everywhere
	}
	for major := 7; major <= 12; major++ {
		for minor := 0; minor <= 40; minor++ {
			for patch := 0; patch <= 60; patch++ {
				if v := (version.Version{Major: major, Minor: minor, Patch: patch}); canon.PropertyDeclared(typ, key, v) {
					return v, true
				}
			}
		}
	}
	return version.Version{}, false
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
