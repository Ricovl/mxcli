// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#953 item 1: a call to a VOID Java/JavaScript action keeps an
// output name in the model and declares no variable, so two of them are not a
// CE0111 — and a nanoflow's variable names are as flat as a microflow's, so a
// real duplicate across if/else branches is. Measured on mxbuild 11.13.0; see
// validate_void_code_calls.go.
package executor

import (
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func javaCall(out, action string) *ast.CallJavaActionStmt {
	return &ast.CallJavaActionStmt{OutputVariable: out, ActionName: qn("M", action)}
}

func jsCall(out, action string) *ast.CallJavaScriptActionStmt {
	return &ast.CallJavaScriptActionStmt{OutputVariable: out, ActionName: qn("M", action)}
}

func mdl063(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL063" {
			out = append(out, v)
		}
	}
	return out
}

// codeActionDecls declares a Java and a JavaScript action of each return kind.
func codeActionDecls() []ast.Statement {
	return []ast.Statement{
		&ast.CreateJavaActionStmt{Name: qn("M", "JaVoid"), ReturnType: ast.DataType{Kind: ast.TypeVoid}},
		&ast.CreateJavaActionStmt{Name: qn("M", "JaBool"), ReturnType: ast.DataType{Kind: ast.TypeBoolean}},
		&ast.CreateJavaScriptActionStmt{Name: qn("M", "JsVoid"), ReturnType: ast.DataType{Kind: ast.TypeVoid}},
		&ast.CreateJavaScriptActionStmt{Name: qn("M", "JsBool"), ReturnType: ast.DataType{Kind: ast.TypeBoolean}},
	}
}

func programWith(flow ast.Statement) *ast.Program {
	return &ast.Program{Statements: append(codeActionDecls(), flow)}
}

func TestMDL063_VoidCallOutputsAreNotDeclarations(t *testing.T) {
	cases := []struct {
		name string
		flow ast.Statement
		want int // MDL063 violations
	}{
		{"microflow: two void java calls, same name", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("R", "JaVoid"), javaCall("R", "JaVoid")}}, 0},
		{"microflow control: two Boolean java calls, same name", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("R", "JaBool"), javaCall("R", "JaBool")}}, 1},
		{"microflow: void call then a declare of the same name", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("V", "JaVoid"),
				&ast.DeclareStmt{Variable: "V", Type: ast.DataType{Kind: ast.TypeString},
					InitialValue: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}}}}, 0},
		{"nanoflow: two void JS calls, same name", &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
			Body: []ast.MicroflowStatement{jsCall("RefreshEntity", "JsVoid"), jsCall("RefreshEntity", "JsVoid")}}, 0},
		{"nanoflow control: two Boolean JS calls, same name", &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
			Body: []ast.MicroflowStatement{jsCall("R", "JsBool"), jsCall("R", "JsBool")}}, 1},
		{"nanoflow: void JS call then a non-void one, same name", &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
			Body: []ast.MicroflowStatement{jsCall("W", "JsVoid"), jsCall("W", "JsBool")}}, 0},
		// An action nobody can resolve keeps counting: guessing void would
		// silence a real CE0111.
		{"microflow: unresolvable action still counts", &ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
			Body: []ast.MicroflowStatement{javaCall("R", "Elsewhere"), javaCall("R", "Elsewhere")}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mdl063(ValidateProgram(programWith(tc.flow), ""))
			if len(got) != tc.want {
				t.Fatalf("MDL063 count = %d, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

// The false negative found alongside: a nanoflow's branches do not open a scope
// either. mxbuild 11.13.0 reports CE0111 for each of these in a nanoflow.
func TestMDL063_NanoflowNamesAreFlowWide(t *testing.T) {
	str := ast.DataType{Kind: ast.TypeString}
	lit := &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}
	cond := &ast.VariableExpr{Name: "C"}
	cases := []struct {
		name   string
		params []ast.MicroflowParam
		body   []ast.MicroflowStatement
		want   int
	}{
		{"non-void call in each if/else branch", nil, []ast.MicroflowStatement{&ast.IfStmt{Condition: cond,
			ThenBody: []ast.MicroflowStatement{jsCall("R", "JsBool")},
			ElseBody: []ast.MicroflowStatement{jsCall("R", "JsBool")}}}, 1},
		{"control: different names per branch", nil, []ast.MicroflowStatement{&ast.IfStmt{Condition: cond,
			ThenBody: []ast.MicroflowStatement{jsCall("R1", "JsBool")},
			ElseBody: []ast.MicroflowStatement{jsCall("R2", "JsBool")}}}, 0},
		{"void call in each branch", nil, []ast.MicroflowStatement{&ast.IfStmt{Condition: cond,
			ThenBody: []ast.MicroflowStatement{jsCall("R", "JsVoid")},
			ElseBody: []ast.MicroflowStatement{jsCall("R", "JsVoid")}}}, 0},
		{"declare in each branch", nil, []ast.MicroflowStatement{&ast.IfStmt{Condition: cond,
			ThenBody: []ast.MicroflowStatement{&ast.DeclareStmt{Variable: "E", Type: str, InitialValue: lit}},
			ElseBody: []ast.MicroflowStatement{&ast.DeclareStmt{Variable: "E", Type: str, InitialValue: lit}}}}, 1},
		{"parameter and declare", []ast.MicroflowParam{{Name: "P", Type: str}},
			[]ast.MicroflowStatement{&ast.DeclareStmt{Variable: "P", Type: str, InitialValue: lit}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nf := &ast.CreateNanoflowStmt{Name: qn("M", "Nf"), Parameters: tc.params, Body: tc.body}
			got := mdl063(ValidateProgram(programWith(nf), ""))
			if len(got) != tc.want {
				t.Fatalf("MDL063 count = %d, want %d: %v", len(got), tc.want, got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Message, "in this nanoflow") {
				t.Errorf("message names the wrong document kind: %s", got[0].Message)
			}
		})
	}
}

// The builder rewrites a String contains/find into a Change Variable, so it
// creates nothing; MDL063 must stay silent on it in a nanoflow as it does in a
// microflow (the kinds it reads were never recorded for a nanoflow).
func TestMDL063_NanoflowStringContainsIsNotAProducer(t *testing.T) {
	str := ast.DataType{Kind: ast.TypeString}
	nf := &ast.CreateNanoflowStmt{Name: qn("M", "Nf"),
		Parameters: []ast.MicroflowParam{{Name: "Hay", Type: str}},
		Body: []ast.MicroflowStatement{
			&ast.DeclareStmt{Variable: "Found", Type: ast.DataType{Kind: ast.TypeBoolean},
				InitialValue: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false}},
			&ast.ListOperationStmt{OutputVariable: "Found", Operation: ast.ListOpContains, InputVariable: "Hay"},
		}}
	if got := mdl063(ValidateNanoflow(nf)); len(got) != 0 {
		t.Fatalf("MDL063 on a String contains: %v", got)
	}
}

// Stored actions are read from the project: Studio Pro's NanoflowCommons
// RefreshEntity is void, and its calls carry `$RefreshEntity`.
func TestVoidCodeActions_ReadsStoredReturnType(t *testing.T) {
	b := &mock.MockBackend{
		ReadJavaScriptActionByNameFunc: func(name string) (*types.JavaScriptAction, error) {
			switch name {
			case "NanoflowCommons.RefreshEntity":
				return &types.JavaScriptAction{ReturnType: &types.VoidType{}}, nil
			case "M.IsStrict":
				return &types.JavaScriptAction{ReturnType: &types.BooleanType{}}, nil
			}
			return nil, nil
		},
		ReadJavaActionByNameFunc: func(name string) (*javaactions.JavaAction, error) {
			switch name {
			case "M.JaVoid":
				// What the modelsdk reader returns for a stored Void.
				return &javaactions.JavaAction{ReturnType: nil}, nil
			case "M.JaVoidTyped":
				return &javaactions.JavaAction{ReturnType: &javaactions.VoidType{}}, nil
			case "M.JaBool":
				return &javaactions.JavaAction{ReturnType: &javaactions.BooleanType{}}, nil
			}
			return nil, nil
		},
	}
	r := newVoidCodeActions(nil, func() backend.FullBackend { return b })
	if !r.isVoid(true, "NanoflowCommons.RefreshEntity") {
		t.Error("stored void JS action not recognised")
	}
	if r.isVoid(true, "M.IsStrict") {
		t.Error("control: a Boolean JS action read as void")
	}
	if !r.isVoid(false, "M.JaVoid") {
		t.Error("stored void Java action not recognised")
	}
	if !r.isVoid(false, "M.JaVoidTyped") {
		t.Error("a Java action with a VoidType return not recognised")
	}
	if r.isVoid(false, "M.JaBool") {
		t.Error("control: a Boolean Java action read as void")
	}
	if r.isVoid(false, "M.Missing") {
		t.Error("an action the project does not have read as void")
	}
}

// The check-time body validator no longer reports duplicate names for a flow
// — MDL063 owns that, flow-wide and void-aware. It kept counting a void JS
// call's output ("already declared in this scope") and scoped names per branch.
func TestValidateFlowBody_LeavesFlowDuplicatesToMDL063(t *testing.T) {
	body := []ast.MicroflowStatement{jsCall("RefreshEntity", "JsVoid"), jsCall("RefreshEntity", "JsVoid")}
	if errs := ValidateNanoflowBody(&ast.CreateNanoflowStmt{Name: qn("M", "Nf"), Body: body}); len(errs) != 0 {
		t.Fatalf("nanoflow body validator still reports duplicates: %v", errs)
	}
	if errs := ValidateMicroflowBody(&ast.CreateMicroflowStmt{Name: qn("M", "Mf"),
		Body: []ast.MicroflowStatement{javaCall("R", "JaVoid"), javaCall("R", "JaVoid")}}); len(errs) != 0 {
		t.Fatalf("microflow body validator still reports duplicates: %v", errs)
	}
	// A rule has no MDL063, so its body validator keeps the check.
	str := ast.DataType{Kind: ast.TypeString}
	lit := &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}
	rule := &ast.CreateRuleStmt{Name: qn("M", "Rule"), Body: []ast.MicroflowStatement{
		&ast.DeclareStmt{Variable: "X", Type: str, InitialValue: lit},
		&ast.DeclareStmt{Variable: "X", Type: str, InitialValue: lit},
	}}
	if errs := ValidateRuleBody(rule); len(errs) == 0 {
		t.Fatal("control: a rule's duplicate declare is no longer reported")
	}
}

// describe's "duplicate output variable … model is invalid" header is false
// for void calls: mxbuild builds two of them clean.
func TestDescribeDuplicateWarning_SkipsVoidCalls(t *testing.T) {
	call := func(id string, x int) *microflows.ActionActivity {
		a := &microflows.ActionActivity{Action: &microflows.JavaScriptActionCallAction{
			JavaScriptAction: "NanoflowCommons.RefreshEntity", OutputVariableName: "RefreshEntity", UseReturnVariable: true}}
		a.ID = model.ID(id)
		a.Position = model.Point{X: x, Y: 100}
		return a
	}
	start := &microflows.StartEvent{}
	start.ID = "start"
	end := &microflows.EndEvent{}
	end.ID = "end"
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, call("a", 100), call("b", 200), end},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "a"},
			{OriginID: "a", DestinationID: "b"},
			{OriginID: "b", DestinationID: "end"},
		},
	}
	if w := duplicateOutputVariableWarnings(oc, func(any) bool { return true }); len(w) != 0 {
		t.Fatalf("void calls warned as duplicates: %v", w)
	}
	if w := duplicateOutputVariableWarnings(oc, func(any) bool { return false }); len(w) != 1 {
		t.Fatalf("control: non-void duplicate not warned: %v", w)
	}
}

// The editor validates on every keystroke and reading an action from the
// project costs ~300ms on PedApp, so FlowRules shares resolutions through a
// CodeActionCache: a second run does not open the project, an entry older than
// the TTL is read again, and a different project starts empty (ako/mxcli#962).
func TestCodeActionCache_SharesProjectAnswersAcrossRuns(t *testing.T) {
	opens := 0
	b := &mock.MockBackend{
		ReadJavaScriptActionByNameFunc: func(name string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{ReturnType: &types.VoidType{}}, nil
		},
	}
	open := func() backend.FullBackend { opens++; return b }
	now := time.Unix(0, 0)
	cache := NewCodeActionCache()
	cache.now = func() time.Time { return now }
	run := func(project string) bool {
		cache.forProject(project)
		r := newVoidCodeActions(nil, open)
		r.shared = cache
		return r.isVoid(true, "M.JsVoid")
	}
	if void := run("a.mpr"); !void || opens != 1 {
		t.Fatalf("first run: void=%v opens=%d, want true/1", void, opens)
	}
	if !run("a.mpr") || opens != 1 {
		t.Errorf("second run re-opened the project (opens=%d)", opens)
	}
	now = now.Add(codeActionCacheTTL + time.Second)
	if run("a.mpr"); opens != 2 {
		t.Errorf("an expired entry was served (opens=%d, want 2)", opens)
	}
	if run("b.mpr"); opens != 3 {
		t.Errorf("another project was served a's answers (opens=%d, want 3)", opens)
	}
}
