// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowActionsOf builds stmts the way exec does and returns the action of
// every action activity, in flow order.
func flowActionsOf(t *testing.T, varTypes map[string]string, stmts ...ast.MicroflowStatement) []microflows.MicroflowAction {
	t.Helper()
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: map[string]string{},
	}
	oc := fb.buildFlowGraph(stmts, nil)
	if len(fb.errors) > 0 {
		t.Fatalf("builder errors: %v", fb.errors)
	}
	var out []microflows.MicroflowAction
	for _, obj := range oc.Objects {
		if a, ok := obj.(*microflows.ActionActivity); ok {
			out = append(out, a.Action)
		}
	}
	return out
}

// `clear $List;` writes a Change list action of type Clear with no value,
// which is what describe reads back as `clear $List;` (ako/mxcli#944).
func TestClearListBuildsChangeListClear(t *testing.T) {
	acts := flowActionsOf(t, map[string]string{"Orders": "List of Sales.Order"},
		&ast.ClearListStmt{List: "Orders"})
	if len(acts) != 1 {
		t.Fatalf("got %d actions, want 1", len(acts))
	}
	cl, ok := acts[0].(*microflows.ChangeListAction)
	if !ok {
		t.Fatalf("action = %T, want *microflows.ChangeListAction", acts[0])
	}
	if cl.Type != microflows.ChangeListTypeClear || cl.ChangeVariable != "Orders" || cl.Value != "" {
		t.Errorf("got Type=%q ChangeVariable=%q Value=%q, want Clear/Orders/\"\"", cl.Type, cl.ChangeVariable, cl.Value)
	}
}

// `set $List = expr` on a LIST variable is the Replace (stored "Set")
// operation of a Change list activity, the shape describe prints it in. A
// Change variable action on a list is CE7247 ("Variable 'A' does not have a
// primitive type"), so routing it there wrote a model mxbuild refuses.
func TestSetOnListVariableBuildsChangeListSet(t *testing.T) {
	acts := flowActionsOf(t, map[string]string{"A": "List of Sales.Order", "B": "List of Sales.Order"},
		&ast.MfSetStmt{Target: "A", Value: &ast.VariableExpr{Name: "B"}})
	if len(acts) != 1 {
		t.Fatalf("got %d actions, want 1", len(acts))
	}
	cl, ok := acts[0].(*microflows.ChangeListAction)
	if !ok {
		t.Fatalf("action = %T, want *microflows.ChangeListAction (a Change variable on a list is CE7247)", acts[0])
	}
	if cl.Type != microflows.ChangeListTypeSet || cl.ChangeVariable != "A" || cl.Value != "$B" {
		t.Errorf("got Type=%q ChangeVariable=%q Value=%q, want Set/A/$B", cl.Type, cl.ChangeVariable, cl.Value)
	}
}

// Control: `set` on a primitive variable stays a Change variable action.
func TestSetOnPrimitiveVariableStaysChangeVariable(t *testing.T) {
	acts := flowActionsOf(t, map[string]string{"N": "Integer"},
		&ast.MfSetStmt{Target: "N", Value: &ast.LiteralExpr{Value: int64(3), Kind: ast.LiteralInteger}})
	if len(acts) != 1 {
		t.Fatalf("got %d actions, want 1", len(acts))
	}
	if _, ok := acts[0].(*microflows.ChangeVariableAction); !ok {
		t.Fatalf("action = %T, want *microflows.ChangeVariableAction", acts[0])
	}
}
