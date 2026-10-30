// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// flowGateCase is one activity a flow body gate names: the statement, the
// last version that cannot store it, its floor, and how check names it.
type flowGateCase struct {
	stmt         ast.MicroflowStatement
	below, floor [3]int
	want         string
	// nanoflow: the activity is also a nanoflow's (a list operation, a loop),
	// so create nanoflow and alter nanoflow are gated on it as well.
	nanoflow bool
}

// flowGatePlaces are where an activity sits in a body: every one of them is
// found (bodyContains walks branches, loops and error handlers).
func flowGatePlaces(st ast.MicroflowStatement) []struct {
	name string
	body []ast.MicroflowStatement
} {
	return []struct {
		name string
		body []ast.MicroflowStatement
	}{
		{"top level", []ast.MicroflowStatement{st}},
		{"in a loop", []ast.MicroflowStatement{&ast.LoopStmt{LoopVariable: "It", ListVariable: "List", Body: []ast.MicroflowStatement{st}}}},
		{"in an else branch", []ast.MicroflowStatement{&ast.IfStmt{HasElse: true, ElseBody: []ast.MicroflowStatement{st}}}},
		{"in a while loop", []ast.MicroflowStatement{&ast.WhileStmt{Body: []ast.MicroflowStatement{st}}}},
		{"in an error handler", []ast.MicroflowStatement{&ast.RetrieveStmt{Variable: "X", Source: ast.QualifiedName{Module: "M", Name: "E"},
			ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingCustom, Body: []ast.MicroflowStatement{st}}}}},
	}
}

// flowGateStatements are the statements that write a body: create, an alter's
// insert, and an alter's replace beside a drop — for a nanoflow too when the
// activity is one.
func flowGateStatements(body []ast.MicroflowStatement, nanoflow bool) []ast.Statement {
	name := ast.QualifiedName{Module: "M", Name: "Flow"}
	stmts := []ast.Statement{
		&ast.CreateMicroflowStmt{Name: name, Body: body},
		&ast.AlterFlowStmt{Name: name, Operations: []*ast.AlterFlowOperation{{Op: ast.AlterFlowInsertAfter, Target: "$X", Body: body}}},
		&ast.AlterFlowStmt{Name: name, Operations: []*ast.AlterFlowOperation{
			{Op: ast.AlterFlowDrop, Target: "$Y"}, {Op: ast.AlterFlowReplace, Target: "$X", Body: body}}},
	}
	if nanoflow {
		stmts = append(stmts,
			&ast.CreateNanoflowStmt{Name: name, Body: body},
			&ast.AlterFlowStmt{Name: name, Nanoflow: true, Operations: []*ast.AlterFlowOperation{{Op: ast.AlterFlowInsertBefore, Target: "$X", Body: body}}})
	}
	return stmts
}

// checkFlowGateCases asserts, for every place and statement, that check -p
// reports the activity below its floor with MDL-VERSION01 naming it and the
// version, that exec's handler refuses with the same text, and that the
// activity is not reported at its floor. (At its floor something else may
// be: a loop is itself gated below 9.0.4.)
func checkFlowGateCases(t *testing.T, cases []flowGateCase) {
	t.Helper()
	for _, c := range cases {
		floor := version.Version{Major: c.floor[0], Minor: c.floor[1], Patch: c.floor[2]}
		wantVersion := "requires Mendix " + floor.String() + "+"
		for _, place := range flowGatePlaces(c.stmt) {
			for _, stmt := range flowGateStatements(place.body, c.nanoflow) {
				prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, stmt}}
				vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
				if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want+" requires") ||
					!strings.Contains(vs[0].Message, wantVersion) || vs[0].Suggestion == "" {
					t.Errorf("%s %s (%T) below its floor %v: want one MDL-VERSION01 naming %q and %q with a hint, got %+v",
						c.want, place.name, stmt, c.below, c.want, wantVersion, vs)
				}
				if err := checkVersionGates(versionCtx(t, c.below[0], c.below[1], c.below[2]), stmt); err == nil ||
					len(vs) == 0 || !strings.Contains(vs[0].Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
					t.Errorf("%s %s (%T): exec's refusal %v differs from check's %+v", c.want, place.name, stmt, err, vs)
				}
				for _, v := range versionGateViolations(versionCtx(t, c.floor[0], c.floor[1], c.floor[2]), prog) {
					if strings.Contains(v.Message, c.want+" requires") {
						t.Errorf("%s %s (%T): reported at its floor %v: %+v", c.want, place.name, stmt, c.floor, v)
					}
				}
			}
		}
	}
}

// ako/mxcli#1811: the microflow activities whose storage types Mendix added
// after 9.0 had no check-time gate, so `check -p` passed them and the write
// refused them part-way through exec (canon.UndeclaredTypeError). Each floor is
// its type's introduction in mendixmodelsdk 4.117.0:
//
//	find / filter by expression                         9.0.3
//	call external action                                9.19.0
//	range                                               9.24.0
//	send rest request                                   10.1.0
//	  … with ( … ), call external action ( arguments )  10.2.0
//	  $Result = …                                       10.4.0
//	  … body $Variable                                  10.5.0
//	transform … with                                    11.4.0
func TestCheckVersionGates_FlowActions1811(t *testing.T) {
	cond := &ast.BinaryExpr{Left: &ast.AttributePathExpr{Variable: "currentObject", Path: []string{"Amount"}},
		Operator: ">", Right: &ast.LiteralExpr{Value: int64(0), Kind: ast.LiteralInteger}}
	op := func(kind ast.ListOperationType, byExpression bool) *ast.ListOperationStmt {
		return &ast.ListOperationStmt{OutputVariable: "Out", Operation: kind, InputVariable: "List", Condition: cond, ByExpression: byExpression}
	}
	rest := ast.QualifiedName{Module: "M", Name: "Service.Operation"}
	checkFlowGateCases(t, []flowGateCase{
		{op(ast.ListOpFind, true), [3]int{9, 0, 2}, [3]int{9, 0, 3}, "find / filter by expression (a list operation)", true},
		{op(ast.ListOpFilter, true), [3]int{9, 0, 2}, [3]int{9, 0, 3}, "find / filter by expression (a list operation)", true},
		// The function form whose condition is not `Member = value`.
		{op(ast.ListOpFilter, false), [3]int{9, 0, 2}, [3]int{9, 0, 3}, "find / filter by expression (a list operation)", true},
		{&ast.CallExternalActionStmt{ServiceName: ast.QualifiedName{Module: "M", Name: "Svc"}, ActionName: "Reset"},
			[3]int{9, 18, 0}, [3]int{9, 19, 0}, "call external action", false},
		{&ast.ListOperationStmt{OutputVariable: "Page", Operation: ast.ListOpRange, InputVariable: "List",
			LimitExpr: &ast.LiteralExpr{Value: int64(10), Kind: ast.LiteralInteger}},
			[3]int{9, 23, 0}, [3]int{9, 24, 0}, "range (a list operation)", true},
		{&ast.SendRestRequestStmt{Operation: rest}, [3]int{9, 24, 30}, [3]int{10, 1, 0}, "send rest request", false},
		{&ast.SendRestRequestStmt{Operation: rest, Parameters: []ast.SendRestParamDef{{Name: "Id", Expression: "1"}}},
			[3]int{10, 1, 0}, [3]int{10, 2, 0}, "send rest request … with ( … )", false},
		{&ast.CallExternalActionStmt{ServiceName: ast.QualifiedName{Module: "M", Name: "Svc"}, ActionName: "Get",
			Arguments: []ast.CallArgument{{Name: "Id", Value: &ast.LiteralExpr{Value: int64(1), Kind: ast.LiteralInteger}}}},
			[3]int{10, 1, 0}, [3]int{10, 2, 0}, "call external action … ( arguments )", false},
		{&ast.SendRestRequestStmt{Operation: rest, OutputVariable: "Response"}, [3]int{10, 3, 0}, [3]int{10, 4, 0},
			"$Result = send rest request …", false},
		{&ast.SendRestRequestStmt{Operation: rest, BodyVariable: "Order"}, [3]int{10, 4, 0}, [3]int{10, 5, 0},
			"send rest request … body $Variable", false},
		{&ast.TransformJsonStmt{OutputVariable: "Out", InputVariable: "Json", Transformation: ast.QualifiedName{Module: "M", Name: "T"}},
			[3]int{11, 3, 0}, [3]int{11, 4, 0}, "transform … with (a data transformer)", false},
	})
}

// A find or filter BY a member writes Microflows$Find / Filter, which every
// version has, and a `find` on a String is a String find() call, not a list
// operation: neither is gated, on 9.0.0. A `find` on a String the statement
// cannot see is (an alter's fragment over a parameter of the stored flow): the
// gate cannot tell it from a list.
func TestCheckVersionGates_FindFilterByMemberIsNotGated(t *testing.T) {
	member := &ast.BinaryExpr{Left: &ast.IdentifierExpr{Name: "Status"}, Operator: "=",
		Right: &ast.LiteralExpr{Value: "Open", Kind: ast.LiteralString}}
	text := &ast.LiteralExpr{Value: "needle", Kind: ast.LiteralString}
	stringFind := &ast.ListOperationStmt{OutputVariable: "Pos", Operation: ast.ListOpFind, InputVariable: "Text", Condition: text}
	for _, c := range []struct {
		name string
		stmt ast.Statement
	}{
		{"find by member", &ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{
			&ast.ListOperationStmt{OutputVariable: "Out", Operation: ast.ListOpFind, InputVariable: "List", Condition: member}}}},
		{"filter by member", &ast.CreateNanoflowStmt{Body: []ast.MicroflowStatement{
			&ast.ListOperationStmt{OutputVariable: "Out", Operation: ast.ListOpFilter, InputVariable: "List", Condition: member}}}},
		{"find on a String parameter", &ast.CreateMicroflowStmt{
			Parameters: []ast.MicroflowParam{{Name: "Text", Type: ast.DataType{Kind: ast.TypeString}}},
			Body:       []ast.MicroflowStatement{stringFind}}},
		{"find on a declared String", &ast.AlterFlowStmt{Operations: []*ast.AlterFlowOperation{{Op: ast.AlterFlowInsertAfter, Target: "$X",
			Body: []ast.MicroflowStatement{&ast.DeclareStmt{Variable: "Text", Type: ast.DataType{Kind: ast.TypeString}}, stringFind}}}}},
	} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, c.stmt}}
		if vs := versionGateViolations(versionCtx(t, 9, 0, 0), prog); len(vs) != 0 {
			t.Errorf("%s reported on 9.0.0: %+v", c.name, vs)
		}
	}
	// Control: the same find over a list is the by-expression operation.
	prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{stringFind}}}}
	if vs := versionGateViolations(versionCtx(t, 9, 0, 0), prog); len(vs) != 1 {
		t.Errorf("a find over an unknown variable whose condition is not Member = value: want one MDL-VERSION01 on 9.0.0, got %+v", vs)
	}
}

// exec without the pre-flight (the REPL, --no-check) refuses an alter that
// inserts one of them before reading the stored flow, as it does #1787's.
func TestExecAlterFlow_RefusesAGatedFlowActionBeforeReading(t *testing.T) {
	stmt := &ast.AlterFlowStmt{Name: ast.QualifiedName{Module: "M", Name: "MF"}, Operations: []*ast.AlterFlowOperation{
		{Op: ast.AlterFlowInsertAfter, Target: "$X", Body: []ast.MicroflowStatement{
			&ast.SendRestRequestStmt{Operation: ast.QualifiedName{Module: "M", Name: "Service.Operation"}}}}}}
	err := execAlterFlow(versionCtx(t, 9, 24, 30), stmt)
	if err == nil || !strings.Contains(err.Error(), "send rest request requires Mendix 10.1.0+ (project is") {
		t.Fatalf("alter microflow inserting send rest request on 9.24.30: %v", err)
	}
}
