// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1495: a pluggable widget's text-template parameter written
// as an expression, in the reporter's own spelling, was stored as an attribute
// path on the context entity with its spaces stripped (v0.24.0):
//
//	CE1613 "The selected attribute 'Repro.Parent.if1=1then'A'else'B'' no longer exists."
//
// Two layers each had to be right: the visitor keeps the expression's source
// text (not the token text, which drops whitespace), and the parameter builder
// stores a value that is not an attribute reference as the Expression (#823).
// The serializer keeping it there is pinned in widgetobj.
func TestPluggableTemplateParam_ExpressionIsStoredAsWritten(t *testing.T) {
	const script = `alter page M.P {
  replace fieldset1 with {
    fieldset fs (legend: '{1}', legendParams: [{1}=if 1 = 1 then 'A' else 'B'])
  }
};`
	const want = "if 1 = 1 then 'A' else 'B'"

	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	alter, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok || len(alter.Operations) != 1 {
		t.Fatalf("statement = %T, want one alter page operation", prog.Statements[0])
	}
	replace, ok := alter.Operations[0].(*ast.ReplaceWidgetOp)
	if !ok || len(replace.NewWidgets) != 1 {
		t.Fatalf("operation = %T, want a replace with one widget", alter.Operations[0])
	}
	raw, ok := lookupProperty(replace.NewWidgets[0].Properties, "legendParams")
	if !ok {
		t.Fatalf("legendParams not on the widget: %v", replace.NewWidgets[0].Properties)
	}
	params, ok := raw.([]ast.ParamAssignmentV3)
	if !ok || len(params) != 1 {
		t.Fatalf("legendParams = %#v, want one parameter assignment", raw)
	}
	if params[0].Value != want {
		t.Fatalf("parsed value = %q, want %q (the source text, spaces kept)", params[0].Value, want)
	}

	pb := templateBindingPB(false)
	pb.entityContext = "M.Job" // a context entity for a wrong binding to land on
	got := pb.buildClientTemplateParams(params)
	if len(got) != 1 {
		t.Fatalf("got %d template parameters, want 1", len(got))
	}
	if got[0].Expression != want || got[0].AttributeRef != "" {
		t.Errorf("Expression=%q AttributeRef=%q, want the expression stored as written and no attribute",
			got[0].Expression, got[0].AttributeRef)
	}
}
