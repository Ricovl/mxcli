// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// `create java action M.J () returns StringTemplate(Text)` passed check and
// exec and was written with a Boolean return: the return-type converter's
// default turned every kind it did not handle into Boolean, silently. A Java
// action cannot return a text template at all — its return type is a
// CodeActions$Type, of which StringTemplateParameterType is not one, and a
// unit patched to carry one does not load in mxbuild 11.13 ("Type
// Mendix.Modeler.CodeActions.StringTemplateParameterType does not contain a
// constructor with a parameter of type Mendix.Modeler.JavaActions.JavaAction").
// The same default swallowed Binary, the Auto* types and `entity <T>`.

func javaReturnScript(ret string) string {
	return "create java action MyFirstModule.JA_Ret (S: String) " + ret + "\nas $$\nreturn null;\n$$;"
}

func TestCreateJavaAction_UnreturnableTypeIsRefusedAtExec(t *testing.T) {
	for _, c := range []struct{ ret, want string }{
		{"returns StringTemplate(Text)", "StringTemplate(Text)"},
		{"returns StringTemplate(Sql)", "StringTemplate(Sql)"},
		{"returns Binary", "Binary"},
		{"returns AutoNumber", "AutoNumber"},
		{"returns entity <T>", "entity <T>"},
	} {
		t.Run(c.ret, func(t *testing.T) {
			p := newCodeActionProject()
			ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))
			err := execCreateJavaAction(ctx, parseOne[*ast.CreateJavaActionStmt](t, javaReturnScript(c.ret)))
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "return") {
				t.Fatalf("err = %v, want a refusal naming the return type %s", err, c.want)
			}
			if p.ja != nil {
				t.Errorf("written with return type %T despite the refusal", p.ja.ReturnType)
			}
		})
	}
}

// The control: a statement with no `returns` still gets Boolean, Studio Pro's
// default for a new Java action — that one default is deliberate.
func TestCreateJavaAction_OmittedReturnTypeIsBoolean(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))
	assertNoError(t, execCreateJavaAction(ctx, parseOne[*ast.CreateJavaActionStmt](t, javaReturnScript(""))))
	if _, ok := p.ja.ReturnType.(*javaactions.BooleanType); !ok {
		t.Errorf("omitted return type written as %T, want *BooleanType", p.ja.ReturnType)
	}
}

func TestValidate_UnreturnableTypeIsReportedAtCheck(t *testing.T) {
	prog, errs := visitor.Build(javaReturnScript("returns StringTemplate(Text)") + "\n" +
		strings.Replace(javaReturnScript("returns String"), "JA_Ret", "JA_Ok", 1) + "\n" +
		"create javascript action MyFirstModule.JSA_Ret () returns Binary\nas $$\nreturn null;\n$$;\n" +
		"create javascript action MyFirstModule.JSA_Tpl () returns StringTemplate(Text)\nas $$\nreturn null;\n$$;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	rules := map[string][]string{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateJavaActionStmt:
			for _, v := range validateJavaActionReturnType(s) {
				rules[s.Name.Name] = append(rules[s.Name.Name], v.RuleID)
			}
		case *ast.CreateJavaScriptActionStmt:
			for _, v := range validateJavaScriptActionTypes(s) {
				rules[s.Name.Name] = append(rules[s.Name.Name], v.RuleID)
			}
		}
	}
	want := map[string]string{
		"JA_Ret":  "MDL-JA03",
		"JA_Ok":   "",
		"JSA_Ret": "MDL-JA03",
		"JSA_Tpl": "MDL-JSA01", // reported once, by the JavaScript rule
	}
	for name, rule := range want {
		got := strings.Join(rules[name], ",")
		if got != rule {
			t.Errorf("%s: rules = %q, want %q", name, got, rule)
		}
	}
}
