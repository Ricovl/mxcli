// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A JavaScript action has no text-template parameter type — Studio Pro offers
// it for Java actions only. Main silently wrote `StringTemplate(Text)` on a
// JavaScript action as a String (describe then printed `S: String`); once the
// shared converter learned StringTemplate for Java actions it wrote a
// CodeActions$StringTemplateParameterType into a JavaScript action instead.
// mxbuild 11.13 accepts both at 0 errors, so neither tells the author. Both
// check and exec now refuse it, through validateJavaScriptActionTypes.

const jsTemplateScript = `create javascript action MyFirstModule.JSA_Tpl (
    S: StringTemplate(Text),
    Q: StringTemplate(Sql)
) returns Boolean
as $$
return true;
$$;`

func TestCreateJavaScriptAction_StringTemplateIsRefusedAtExec(t *testing.T) {
	p := newCodeActionProject()
	ctx, _ := newMockCtx(t, withBackend(p.mb), withHierarchy(p.h))

	err := execCreateJavaScriptAction(ctx, parseOne[*ast.CreateJavaScriptActionStmt](t, jsTemplateScript))
	if err == nil {
		t.Fatal("expected a refusal: a JavaScript action has no StringTemplate parameter type")
	}
	for _, want := range []string{"parameter S", "parameter Q", "StringTemplate(Text)", "StringTemplate(Sql)", "Java action"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err.Error(), want)
		}
	}
	if p.jsa != nil {
		t.Error("the action was written despite the refusal")
	}
}

func TestCreateJavaScriptAction_StringTemplateIsRefusedAtCheck(t *testing.T) {
	prog, errs := visitor.Build(jsTemplateScript + `
create javascript action MyFirstModule.JSA_Ret () returns StringTemplate(Text)
as $$
return null;
$$;
create javascript action MyFirstModule.JSA_Ok (S: String) returns String
as $$
return S;
$$;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var got []string
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateJavaScriptActionStmt); ok {
			for _, v := range validateJavaScriptActionTypes(s) {
				if v.RuleID != "MDL-JSA01" {
					t.Errorf("rule = %s, want MDL-JSA01", v.RuleID)
				}
				got = append(got, v.Message)
			}
		}
	}
	if len(got) != 3 {
		t.Fatalf("violations = %d (%v), want 3: S and Q of JSA_Tpl, the return type of JSA_Ret", len(got), got)
	}
	if !strings.Contains(got[2], "JSA_Ret") || !strings.Contains(got[2], "return type") {
		t.Errorf("return-type violation = %q", got[2])
	}
}
