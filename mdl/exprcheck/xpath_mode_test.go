// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"reflect"
	"testing"
)

// An XPath path from the queried entity, with a predicate on one segment,
// parses whole in XPath mode (ako/mxcli#1439). Before, the parse stopped at
// the first `/` and kept only `Mod.A`.
func TestXPathModeParsesEntityRootedPaths(t *testing.T) {
	ctx := NewSyntaxContext("", "")
	ctx.XPath = true
	expr, hs := NewParser().Parse(`Mod.A/Mod.E[Code = 'y']/Name = 'x'`, ctx)
	for _, h := range hs {
		if h.Code == "E014" {
			t.Fatalf("trailing tokens left: %+v", h)
		}
	}
	bin, ok := expr.(*BinExpr)
	if !ok {
		t.Fatalf("root = %T, want *BinExpr", expr)
	}
	path, ok := bin.L.(*AttributePathExpr)
	if !ok {
		t.Fatalf("left = %T, want *AttributePathExpr", bin.L)
	}
	if path.Variable != "" || !reflect.DeepEqual(path.Path, []string{"Mod.A", "Mod.E", "Name"}) {
		t.Errorf("path = %q %v", path.Variable, path.Path)
	}
	if len(path.Predicates) != 3 || path.Predicates[0] != nil || path.Predicates[2] != nil {
		t.Fatalf("predicates = %#v, want one on segment 1", path.Predicates)
	}
	if _, ok := path.Predicates[1].(*BinExpr); !ok {
		t.Errorf("predicate on Mod.E = %T, want *BinExpr", path.Predicates[1])
	}
}

// Control: a microflow expression is not XPath. `Mod.A/Mod.E` is not a valid
// expression, so the expression parser must keep reporting it, not quietly
// accept it as a path.
func TestExpressionModeKeepsRejectingEntityRootedPaths(t *testing.T) {
	expr, hs := NewParser().Parse(`Mod.A/Mod.E`, NewSyntaxContext("", ""))
	if _, ok := expr.(*QNameExpr); !ok {
		t.Errorf("root = %T, want *QNameExpr", expr)
	}
	found := false
	for _, h := range hs {
		found = found || h.Code == "E014"
	}
	if !found {
		t.Errorf("no E014 for an entity-rooted path in an expression: %+v", hs)
	}
}
