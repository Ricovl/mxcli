// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `clear $List;` is the Clear operation of a Change list activity. describe
// already printed it for a Studio Pro-authored Clear, so before the statement
// existed a round trip of such a flow failed to parse (ako/mxcli#944, item 1).
func TestClearListStatementParses(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow"} {
		input := `create ` + kind + ` Sales.ResetOrders ($Order: Sales.Order)
begin
  $Orders = create list of Sales.Order;
  add $Order to $Orders;
  @caption 'Empty it'
  clear $Orders;
end;`

		prog, errs := Build(input)
		if len(errs) > 0 {
			t.Fatalf("%s: parse errors: %v", kind, errs)
		}
		var body []ast.MicroflowStatement
		switch s := prog.Statements[0].(type) {
		case *ast.CreateMicroflowStmt:
			body = s.Body
		case *ast.CreateNanoflowStmt:
			body = s.Body
		default:
			t.Fatalf("%s: statement = %T", kind, prog.Statements[0])
		}
		clear, ok := body[2].(*ast.ClearListStmt)
		if !ok {
			t.Fatalf("%s: body[2] = %T, want *ast.ClearListStmt", kind, body[2])
		}
		if clear.List != "Orders" {
			t.Errorf("%s: List = %q, want Orders", kind, clear.List)
		}
		if clear.Annotations == nil || clear.Annotations.Caption != "Empty it" {
			t.Errorf("%s: the @caption annotation was not attached: %#v", kind, clear.Annotations)
		}
	}
}

// Clear takes the list variable and nothing else: Mendix stores no value for it.
func TestClearListRefusesAValue(t *testing.T) {
	_, errs := Build(`create microflow Sales.M ($A: List of Sales.Order)
begin
  clear $A with $B;
end;`)
	if len(errs) == 0 {
		t.Fatal("clear with a value parsed; Clear has no operand")
	}
	if !strings.Contains(errs[0].Error(), "line 3") {
		t.Errorf("error not on the clear line: %v", errs[0])
	}
}
