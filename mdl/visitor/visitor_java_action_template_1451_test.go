// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1451: a Java action's text-template parameter is filled the way
// log and show message fill a template — `Subject = 'Order {1}' with ({1} =
// $Order/Number)`. The `with` belongs to that one argument.
func TestCallJavaActionTemplateArgument(t *testing.T) {
	prog, errs := Build(`create microflow M.F ($O: M.E)
begin
  call java action M.J(Subject = 'Order {1} of {2}' with ({1} = $O/Number, {2} = $O/Total), Event = $O);
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	call := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.CallJavaActionStmt)
	if len(call.Arguments) != 2 {
		t.Fatalf("arguments = %d, want 2", len(call.Arguments))
	}
	subject := call.Arguments[0]
	if subject.Name != "Subject" || len(subject.TemplateParams) != 2 {
		t.Fatalf("Subject = %+v, want two template parameters", subject)
	}
	if lit, ok := subject.Value.(*ast.LiteralExpr); !ok || lit.Value != "Order {1} of {2}" {
		t.Errorf("Subject text = %#v", subject.Value)
	}
	if p := subject.TemplateParams[1]; p.Index != 2 || p.Value == nil {
		t.Errorf("{2} = %+v", p)
	}
	if len(call.Arguments[1].TemplateParams) != 0 {
		t.Errorf("Event took the template's parameters: %+v", call.Arguments[1])
	}
}

// Only a Java action has text-template parameters.
func TestTemplateArgumentRefusedOutsideCallJavaAction(t *testing.T) {
	for _, call := range []string{
		"call microflow M.G(Text = 'a {1}' with ({1} = $O));",
		"call javascript action M.JS(Text = 'a {1}' with ({1} = $O));",
	} {
		_, errs := Build("create microflow M.F ($O: M.E) begin " + call + " end;")
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		if !strings.Contains(strings.Join(msgs, "\n"), "only call java action has") {
			t.Errorf("%s: errors = %v, want the text-template refusal", call, msgs)
		}
	}
}
