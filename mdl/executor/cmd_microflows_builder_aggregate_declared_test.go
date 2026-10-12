// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1413: `$Sum = sum $List by Amount; set $Sum = $Sum + 1;`
// passed `check -p` but `exec` refused it with "variable 'Sum' is not
// declared" — the build path never registered an aggregate's output variable,
// while the validate path did.
func TestAggregateOutputIsDeclaredForSet(t *testing.T) {
	cases := []struct {
		name string
		stmt *ast.AggregateListStmt
		want string // declaredVars type, "" = only require it be declared
	}{
		{"sum", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateSum, InputVariable: "List", Attribute: "Amount"}, ""},
		{"count", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateCount, InputVariable: "List"}, "Integer"},
		{"average", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateAverage, InputVariable: "List", Attribute: "Amount"}, "Decimal"},
		{"maximum", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateMaximum, InputVariable: "List", Attribute: "Amount"}, ""},
		{"any", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateAny, InputVariable: "List",
			IsExpression: true, Expression: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}}, "Boolean"},
		{"reduce", &ast.AggregateListStmt{OutputVariable: "Sum", Operation: ast.AggregateReduce, InputVariable: "List",
			IsExpression: true, Expression: &ast.VariableExpr{Name: "currentResult"},
			InitialValue: &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: int64(0)},
			ReturnType:   &ast.DataType{Kind: ast.TypeInteger}}, "Integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []ast.MicroflowStatement{
				tc.stmt,
				&ast.MfSetStmt{Target: "Sum", Value: &ast.BinaryExpr{
					Left:     &ast.VariableExpr{Name: "Sum"},
					Operator: "+",
					Right:    &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: int64(1)},
				}},
			}
			fb := &flowBuilder{
				posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing,
				varTypes:     map[string]string{"List": "List of ModA.EntA"},
				declaredVars: map[string]string{},
			}
			fb.buildFlowGraph(body, nil)
			if errs := fb.GetErrors(); len(errs) > 0 {
				t.Fatalf("build: %s", strings.Join(errs, "; "))
			}
			if tc.want != "" && fb.declaredVars["Sum"] != tc.want {
				t.Errorf("declared type = %q, want %q", fb.declaredVars["Sum"], tc.want)
			}
		})
	}
}
