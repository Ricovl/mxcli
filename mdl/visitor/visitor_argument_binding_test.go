// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R4 (ako/mxcli#751): every call site binds an argument as `Param =
// expression`, and every text template as `with ({1} = expression)`. Each old
// spelling must record its code, and build exactly the statement its canonical
// form builds — the proof that it is an alias and not a change of meaning.
func TestArgumentBindingAliases(t *testing.T) {
	const page = "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $O) { %s } };"
	mf := func(body string) string {
		return "create microflow M.F ($O: M.E, $N: String) begin " + body + " end;"
	}
	wf := func(body string) string {
		return "create workflow M.W parameter $WorkflowContext: M.E begin " + body + " end workflow;"
	}
	pg := func(widget string) string { return strings.Replace(page, "%s", widget, 1) }
	cases := []struct {
		name, old, canonical string
		codes                []string
	}{
		{"call microflow", mf("call microflow M.G($Order = $O, Force = false);"),
			mf("call microflow M.G(Order = $O, Force = false);"), []string{deprecation.DollarArgumentName}},
		{"call nanoflow", mf("$R = call nanoflow M.G($Order = $O);"),
			mf("$R = call nanoflow M.G(Order = $O);"), []string{deprecation.DollarArgumentName}},
		{"call java action", mf("$R = call java action M.J($Amount = 1);"),
			mf("$R = call java action M.J(Amount = 1);"), []string{deprecation.DollarArgumentName}},
		{"keyword-named parameter is quoted only when it must be", mf("call microflow M.G($Page = $O);"),
			mf("call microflow M.G(Page = $O);"), []string{deprecation.DollarArgumentName}},
		// ako/mxcli#533: `Param: e` is the same alias at every call site, not only
		// show page and page actions.
		{"call microflow colon", mf("call microflow M.G(Order: $O, Force: false);"),
			mf("call microflow M.G(Order = $O, Force = false);"),
			[]string{deprecation.ColonArgument, deprecation.ColonArgument}},
		{"call nanoflow colon", mf("$R = call nanoflow M.G(Order: $O);"),
			mf("$R = call nanoflow M.G(Order = $O);"), []string{deprecation.ColonArgument}},
		{"call java action colon", mf("$R = call java action M.J(Amount: 1);"),
			mf("$R = call java action M.J(Amount = 1);"), []string{deprecation.ColonArgument}},
		{"keyword-named parameter colon", mf("call microflow M.G(Page: $O);"),
			mf("call microflow M.G(Page = $O);"), []string{deprecation.ColonArgument}},
		{"send rest request", mf("$R = send rest request M.C.Get with ($id = $N);"),
			mf("$R = send rest request M.C.Get with (id = $N);"), []string{deprecation.DollarArgumentName}},
		{"show page dollar", mf("show page M.P($Order = $O);"),
			mf("show page M.P(Order = $O);"), []string{deprecation.DollarArgumentName}},
		{"show page colon", mf("show page M.P(Order: $O);"),
			mf("show page M.P(Order = $O);"), []string{deprecation.ColonArgument}},
		{"button action colon", pg("actionbutton b (Caption: 'Go', Action: call microflow M.G(Order: $currentObject))"),
			pg("actionbutton b (Caption: 'Go', Action: call microflow M.G(Order = $currentObject))"), []string{deprecation.ColonArgument}},
		{"button action dollar", pg("actionbutton b (Caption: 'Go', Action: call nanoflow M.G($Order = $currentObject))"),
			pg("actionbutton b (Caption: 'Go', Action: call nanoflow M.G(Order = $currentObject))"), []string{deprecation.DollarArgumentName}},
		{"show_page action colon", pg("actionbutton b (Caption: 'Go', Action: show page M.Q(Order: $currentObject))"),
			pg("actionbutton b (Caption: 'Go', Action: show page M.Q(Order = $currentObject))"), []string{deprecation.ColonArgument}},
		{"data source colon", pg("listview lv (DataSource: microflow M.DS(Order: $O, Limit: 10)) { }"),
			pg("listview lv (DataSource: microflow M.DS(Order = $O, Limit = 10)) { }"),
			[]string{deprecation.ColonArgument, deprecation.ColonArgument}},
		{"workflow call microflow", wf("call microflow M.G as act1 caption 'Go' with (M.G.Order = '$WorkflowContext');"),
			wf("call microflow M.G(Order = $WorkflowContext) as act1 caption 'Go';"), []string{deprecation.WorkflowStringArgument}},
		{"workflow call workflow", wf("call workflow M.Sub caption 'Sub' with (Order = '$WorkflowContext/M.E_Other');"),
			wf("call workflow M.Sub(Order = $WorkflowContext/M.E_Other) caption 'Sub';"), []string{deprecation.WorkflowStringArgument}},
		{"workflow expression with a string inside", wf("call microflow M.G with (Label = 'if $WorkflowContext/Name = ''x'' then ''a'' else ''b''');"),
			wf("call microflow M.G(Label = if $WorkflowContext/Name = 'x' then 'a' else 'b');"), []string{deprecation.WorkflowStringArgument}},
		{"show message objects", mf("show message 'Hi {1} {2}' type Warning objects [$N, $O/Name] blocking;"),
			mf("show message 'Hi {1} {2}' type Warning with ({1} = $N, {2} = $O/Name) blocking;"), []string{deprecation.PositionalTemplateArguments}},
		{"validation feedback objects", mf("validation feedback $O/Name message '{1} is wrong' objects [$N];"),
			mf("validation feedback $O/Name message '{1} is wrong' with ({1} = $N);"), []string{deprecation.PositionalTemplateArguments}},
		{"log parameters", mf("log info 'a {1} {2}' parameters ['x', 2];"),
			mf("log info 'a {1} {2}' with ({1} = 'x', {2} = 2);"), []string{deprecation.PositionalTemplateArguments}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, c.codes) {
				t.Errorf("old form recorded %v, want %v", got, c.codes)
			}
			for _, d := range old.Deprecations {
				if d.Fix == nil {
					t.Errorf("%s at line %d has no rewrite: %s", d.Code, d.Line, d.NoFix)
				}
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical form recorded %v, want none", got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("the two forms build different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The canonical workflow argument stores the bare expression's text, exactly
// what the string form stored.
func TestWorkflowCallArgumentStoresTheExpression(t *testing.T) {
	prog := mustBuild(t, "create workflow M.W parameter $WorkflowContext: M.E begin "+
		"call microflow M.G(Order = $WorkflowContext, Count = 1 + 2); end workflow;")
	wf := prog.Statements[0].(*ast.CreateWorkflowStmt)
	cm := wf.Activities[0].(*ast.WorkflowCallMicroflowNode)
	want := []ast.WorkflowParameterMappingNode{
		{Parameter: "Order", Expression: "$WorkflowContext"},
		{Parameter: "Count", Expression: "1 + 2"},
	}
	if !reflect.DeepEqual(cm.ParameterMappings, want) {
		t.Errorf("ParameterMappings = %+v, want %+v", cm.ParameterMappings, want)
	}
}

// A workflow string argument whose content is not an expression that reads
// back as itself keeps its string: the upgrade reports it rather than
// changing the stored expression.
func TestWorkflowStringArgumentWithoutBareFormHasNoRewrite(t *testing.T) {
	for _, arg := range []string{"' $WorkflowContext'", "''", "'$WorkflowContext +'"} {
		prog := mustBuild(t, "create workflow M.W parameter $WorkflowContext: M.E begin "+
			"call microflow M.G with (Order = "+arg+"); end workflow;")
		if len(prog.Deprecations) != 1 || prog.Deprecations[0].Fix != nil || prog.Deprecations[0].NoFix == "" {
			t.Errorf("%s: deprecations = %+v, want one with no rewrite and a reason", arg, prog.Deprecations)
		}
	}
}

func TestWorkflowCallWithBothArgumentListsIsAnError(t *testing.T) {
	_, errs := Build("create workflow M.W parameter $WorkflowContext: M.E begin " +
		"call microflow M.G(Order = $WorkflowContext) with (Order = '$WorkflowContext'); end workflow;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "two argument lists") {
		t.Errorf("errs = %v, want the two-lists error", errs)
	}
}

// with ({n} = …) on a show message is stored by number, and the numbers must
// be 1..N: the model holds a list, so a gap has nowhere to go.
func TestTemplateArgumentsByNumber(t *testing.T) {
	prog := mustBuild(t, "create microflow M.F ($A: String, $B: String) begin "+
		"show message '{1} {2}' with ({2} = $B, {1} = $A); end;")
	sm := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ShowMessageStmt)
	if len(sm.TemplateArgs) != 2 || !strings.Contains(fmtExpr(sm.TemplateArgs[0]), "A") {
		t.Errorf("TemplateArgs = %#v, want $A then $B", sm.TemplateArgs)
	}
	for _, bad := range []string{"with ({1} = $A, {3} = $B)", "with ({1} = $A, {1} = $B)", "with ({2} = $A)"} {
		_, errs := Build("create microflow M.F ($A: String, $B: String) begin show message 'x' " + bad + "; end;")
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), "numbered {1}") {
			t.Errorf("%s: errs = %v, want the numbering error", bad, errs)
		}
	}
}

func fmtExpr(e ast.Expression) string {
	if v, ok := e.(*ast.VariableExpr); ok {
		return v.Name
	}
	return reflect.ValueOf(e).String()
}

func TestBareExpression(t *testing.T) {
	for expr, want := range map[string]bool{
		"$WorkflowContext":          true,
		"$WorkflowContext/M.A/Name": true,
		"1 + 2":                     true,
		"'a''b'":                    true,
		"":                          false,
		" $WorkflowContext":         false,
		"$WorkflowContext +":        false,
		"$WorkflowContext; drop x":  false,
	} {
		if got := BareExpression(expr); got != want {
			t.Errorf("BareExpression(%q) = %v, want %v", expr, got, want)
		}
	}
}

func TestParameterNameSpelling(t *testing.T) {
	for name, want := range map[string]string{
		"Order": "Order",
		"Page":  "Page",
		"with":  "with",
		"a b":   `"a b"`,
	} {
		if got := ParameterNameSpelling(name); got != want {
			t.Errorf("ParameterNameSpelling(%q) = %q, want %q", name, got, want)
		}
	}
}

// ako/mxcli#533, #569: an argument written by position — `microflow M.DS($O)` —
// is refused with an error that points at the argument, not at the property
// keyword before it, and names the form that works: `Param = $O`. Positional
// binding is not an alias under any language version, because the parameter it
// means cannot be known from the script alone.
func TestPositionalArgumentIsRefusedAtTheArgument(t *testing.T) {
	const page = "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $O) { %s } };"
	pg := func(widget string) string { return strings.Replace(page, "%s", widget, 1) }
	mf := func(body string) string {
		return "create microflow M.F ($O: M.E) begin " + body + " end;"
	}
	for _, src := range []string{
		pg("listview lv (DataSource: microflow M.DS($O)) { }"),
		pg("listview lv (DataSource: nanoflow M.DS($O)) { }"),
		pg("actionbutton b (Caption: 'Go', Action: call microflow M.G($O))"),
		mf("call microflow M.G($O);"),
		mf("$R = call nanoflow M.G($O);"),
		mf("show page M.P($O);"),
		"mdl 1;\n" + pg("listview lv (DataSource: microflow M.DS($O)) { }"),
	} {
		t.Run(src, func(t *testing.T) {
			_, errs := Build(src)
			if len(errs) == 0 {
				t.Fatal("a positional argument was accepted")
			}
			lines := strings.Split(src, "\n")
			line := len(lines)
			col := strings.LastIndex(lines[line-1], "($O)") + 1
			want := fmt.Sprintf("line %d:%d", line, col)
			msg := errs[0].Error()
			if !strings.Contains(msg, want) {
				t.Errorf("error %q does not point at the argument (%s)", msg, want)
			}
			if !strings.Contains(msg, "Param = $O") {
				t.Errorf("error %q does not name the named form `Param = $O`", msg)
			}
		})
	}
}
