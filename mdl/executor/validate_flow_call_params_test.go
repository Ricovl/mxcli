// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#953 item 7: an argument naming no parameter of the called flow is
// CE1613 "The selected parameter 'M.F.X' no longer exists" in mxbuild 11.13.0,
// for call microflow and call nanoflow alike. Java and JavaScript action calls
// were checked under --references; flow calls were not.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func flowCallScript(call ast.MicroflowStatement, targetParams []ast.MicroflowParam, nano bool) (*ast.Program, []ast.MicroflowStatement) {
	body := []ast.MicroflowStatement{call}
	var target, caller ast.Statement
	if nano {
		target = &ast.CreateNanoflowStmt{Name: qn("M", "Target"), Parameters: targetParams}
		caller = &ast.CreateNanoflowStmt{Name: qn("M", "Caller"), Body: body}
	} else {
		target = &ast.CreateMicroflowStmt{Name: qn("M", "Target"), Parameters: targetParams}
		caller = &ast.CreateMicroflowStmt{Name: qn("M", "Caller"), Body: body}
	}
	return &ast.Program{Statements: []ast.Statement{target, caller}}, body
}

func TestValidate_FlowCallParameterNames(t *testing.T) {
	str := ast.DataType{Kind: ast.TypeString}
	arg := func(name string) []ast.CallArgument {
		return []ast.CallArgument{{Name: name, Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}}}
	}
	one := []ast.MicroflowParam{{Name: "Input", Type: str}}
	cases := []struct {
		name    string
		call    ast.MicroflowStatement
		params  []ast.MicroflowParam
		nano    bool
		wantErr string
	}{
		{"call microflow, unknown name", &ast.CallMicroflowStmt{MicroflowName: qn("M", "Target"), Arguments: arg("Bogus")},
			one, false, `microflow M.Target has no parameter "Bogus" (declared parameters: Input)`},
		{"control: call microflow, declared name", &ast.CallMicroflowStmt{MicroflowName: qn("M", "Target"), Arguments: arg("Input")},
			one, false, ""},
		{"call nanoflow, unknown name", &ast.CallNanoflowStmt{NanoflowName: qn("M", "Target"), Arguments: arg("Bogus")},
			one, true, `nanoflow M.Target has no parameter "Bogus"`},
		{"control: call nanoflow, declared name", &ast.CallNanoflowStmt{NanoflowName: qn("M", "Target"), Arguments: arg("Input")},
			one, true, ""},
		{"call microflow taking no parameters, with an argument", &ast.CallMicroflowStmt{MicroflowName: qn("M", "Target"), Arguments: arg("Bogus")},
			nil, false, `(declared parameters: none)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := newMockCtx(t)
			prog, body := flowCallScript(tc.call, tc.params, tc.nano)
			sc := newScriptContext()
			sc.collectDefinitions(prog)
			errs := validateFlowBodyReferences(ctx, body, sc)
			if tc.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tc.wantErr) || !strings.Contains(errs[0], "CE1613") {
				t.Fatalf("errors = %v, want one containing %q", errs, tc.wantErr)
			}
		})
	}
}

// A flow's body error used to be returned INSTEAD of its reference errors, so a
// call to a parameter the action does not have (CE1613) stayed hidden behind
// it until the body was fixed — the shape #953 reported.
func TestValidateWithContext_BodyErrorDoesNotHideReferenceErrors(t *testing.T) {
	ctx, _ := newMockCtx(t)
	mf := &ast.CreateMicroflowStmt{Name: qn("M", "Caller"), Body: []ast.MicroflowStatement{
		&ast.MfSetStmt{Target: "Undeclared", Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}},
		&ast.CallJavaActionStmt{ActionName: qn("M", "Ja"), Arguments: []ast.CallArgument{
			{Name: "email", Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "a@b.c"}}}},
	}}
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateJavaActionStmt{Name: qn("M", "Ja"), Parameters: []ast.JavaActionParam{{Name: "EmailAddress"}}},
		mf,
	}}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	err := validateWithContext(ctx, mf, sc)
	if err == nil {
		t.Fatal("no error for a body error plus an unknown parameter")
	}
	for _, want := range []string{"is not declared", `has no parameter "email"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}
