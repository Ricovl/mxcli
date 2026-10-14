// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1406. Studio Pro stores a text-template parameter written
// in its expression editor as an Expression — `$currentObject/Name`,
// `$ChatContext/Topic` — and describe prints that expression back as written.
// check then refused it:
//
//	ContentParams parameter {1} is written as "$currentObject/CollectionIdentifierLabel"
//	— … Write "CollectionIdentifierLabel". … [MDL-WIDGET24]
//
// because the writer read `$var/Attr` as an attribute binding and kept the
// prefix in the attribute name (CE1613, #1046). The writer was the wrong side:
// `$var/Attr` is Mendix expression syntax, and the parameter position takes an
// expression (Forms$ClientTemplateParameter.Expression, since 9.0.1). Stored as
// the expression it is, it is Studio Pro's own model, and MDL-WIDGET24 has
// nothing left to prevent.

func TestTemplateParam_VariableRootedPathIsStoredAsExpression(t *testing.T) {
	for _, tc := range []struct {
		value    string
		wantExpr bool
	}{
		{"$currentObject/Name", true},
		{"$Task/Title", true},
		{"$Task/M.Job_Owner/Name", true},
		// Controls: the binding spellings stay bindings.
		{"Title", false},
		{"$Task.Title", false},
		{"M.Job_Owner/Name", false},
		// `$currentObject/` before an association path is stripped and bound
		// (#1046 measured it building clean); it stays a binding.
		{"$currentObject/M.Job_Owner/Name", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got := templateBindingPB(false).buildClientTemplateParams(
				[]ast.ParamAssignmentV3{{Index: 1, Value: tc.value}})
			if len(got) != 1 {
				t.Fatalf("got %d params, want 1", len(got))
			}
			p := got[0]
			if tc.wantExpr {
				if p.Expression != tc.value || p.AttributeRef != "" {
					t.Errorf("%s: Expression=%q AttributeRef=%q, want the expression stored as written",
						tc.value, p.Expression, p.AttributeRef)
				}
				return
			}
			if p.Expression != "" {
				t.Errorf("%s: stored as Expression %q, want an attribute binding", tc.value, p.Expression)
			}
		})
	}
}

// The check side of the same round trip: what describe prints for a Studio
// Pro expression parameter — the reporter's blank-app reproduction — passes
// the no-project check that `mxcli check` runs.
func TestTemplateParam_VariableRootedPathPassesCheck(t *testing.T) {
	const script = `create page ModA.Pg_A (Title: 'A', Layout: Atlas_Core.Atlas_Default, Params: ( $EntA: ModA.EntA )) {
  dataview dv1 (DataSource: $EntA) {
    dynamictext t1 (Content: '{1}', ContentParams: ({1} = $currentObject/Name))
    dynamictext t2 (Content: '{1}', ContentParams: ({1} = $EntA/Name))
    actionbutton b1 (Caption: '{1}', CaptionParams: ({1} = $EntA/Name), Action: close page)
  }
};`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError {
			t.Errorf("check refused a Studio Pro expression parameter: [%s] %s", v.RuleID, v.Message)
		}
	}
}
