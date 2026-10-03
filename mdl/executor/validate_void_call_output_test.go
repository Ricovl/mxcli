// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func mdl093(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == voidCallOutputRule {
			out = append(out, v)
		}
	}
	return out
}

func logOf(name string) *ast.LogStmt {
	return &ast.LogStmt{Level: ast.LogInfo, Message: &ast.BinaryExpr{
		Left: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}, Operator: "+",
		Right: &ast.VariableExpr{Name: name}}}
}

// ako/mxcli#962 item 1: a void call's output name declares nothing, so reading
// it is CE0109 "Undefined variable" (mxbuild 11.13.0, measured on a PedApp copy
// — see validate_void_call_output.go). Every "want 0" row is a control.
func TestMDL093_ReadOfAVoidCallOutput(t *testing.T) {
	str := ast.DataType{Kind: ast.TypeString}
	lit := &ast.LiteralExpr{Kind: ast.LiteralString, Value: "b"}
	cond := &ast.VariableExpr{Name: "C"}
	jsArg := func(out, action, arg string) *ast.CallJavaScriptActionStmt {
		c := jsCall(out, action)
		c.Arguments = []ast.CallArgument{{Name: "p", Value: &ast.VariableExpr{Name: arg}}}
		return c
	}
	cases := []struct {
		name string
		flow ast.Statement
		want int
	}{
		{"microflow: void java call, then a log reads it", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"), logOf("V")}}, 1},
		{"control: non-void java call, then a log reads it", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaBool"), logOf("V")}}, 0},
		{"control: void call, then a declare of the name, then a read", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"),
				&ast.DeclareStmt{Variable: "V", Type: str, InitialValue: lit}, logOf("V")}}, 0},
		{"control: a parameter of the same name", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Parameters: []ast.MicroflowParam{{Name: "V", Type: str}},
			Body:       []ast.MicroflowStatement{javaCall("V", "JaVoid"), logOf("V")}}, 0},
		{"control: an unresolvable action is never reported", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "Elsewhere"), logOf("V")}}, 0},
		{"control: void call whose name is never read", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"), logOf("Other")}}, 0},
		{"microflow: the read sits in a branch", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Parameters: []ast.MicroflowParam{{Name: "C", Type: ast.DataType{Kind: ast.TypeBoolean}}},
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"),
				&ast.IfStmt{Condition: cond, ThenBody: []ast.MicroflowStatement{logOf("V")}}}}, 1},
		{"nanoflow: void JS call, then its name as an argument", &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
			Body: []ast.MicroflowStatement{jsCall("V", "JsVoid"), jsArg("S", "JsVoid", "V")}}, 1},
		{"nanoflow control: non-void JS call, then its name as an argument", &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
			Body: []ast.MicroflowStatement{jsCall("V", "JsBool"), jsArg("S", "JsVoid", "V")}}, 0},
		{"one report per name, however often it is read", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"), logOf("V"), logOf("V")}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mdl093(ValidateProgram(programWith(tc.flow), ""))
			if len(got) != tc.want {
				t.Fatalf("MDL093 count = %d, want %d: %v", len(got), tc.want, got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Message, "CE0109") {
				t.Errorf("message does not name the build error: %s", got[0].Message)
			}
		})
	}
}

// The editor's "possibly void" policy (unknownIsVoid) spares an unresolvable
// call from MDL063, but must not turn a read of its output into MDL093: only a
// call KNOWN to be void declares nothing.
func TestMDL093_PossiblyVoidIsNotKnownVoid(t *testing.T) {
	mf := &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
		Body: []ast.MicroflowStatement{javaCall("V", "Elsewhere"), javaCall("V", "Elsewhere"), logOf("V")}}
	voids := newVoidCodeActions(nil, nil)
	voids.unknownIsVoid = true
	vs := validateMicroflowWith(mf, voids)
	if got := mdl093(vs); len(got) != 0 {
		t.Errorf("MDL093 on a call that is only possibly void: %v", got)
	}
	if got := mdl063(vs); len(got) != 0 {
		t.Errorf("MDL063 under unknownIsVoid on an unresolvable pair: %v", got)
	}
	// Control: the default policy still counts the pair.
	if got := mdl063(validateMicroflowWith(mf, newVoidCodeActions(nil, nil))); len(got) != 1 {
		t.Errorf("control: MDL063 = %v, want the unresolvable pair reported", got)
	}
}
