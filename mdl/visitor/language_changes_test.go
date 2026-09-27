// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// LanguageChanges must list every langver.Change this package declares: the
// upgrade's registry test holds each listed change to a rewrite or a reason it
// has none, so a change left off the list would escape it.
func TestLanguageChangesListsEveryDeclaredChange(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range LanguageChanges() {
		listed[c.Code] = true
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	declared := 0
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Change" || len(lit.Elts) == 0 {
				return true
			}
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if k, _ := kv.Key.(*ast.Ident); !ok || k == nil || k.Name != "Code" {
					continue
				}
				if v, ok := kv.Value.(*ast.BasicLit); ok {
					declared++
					code := strings.Trim(v.Value, `"`)
					if !listed[code] {
						t.Errorf("%s declares the language change %s, which LanguageChanges does not list", e.Name(), code)
					}
				}
			}
			return true
		})
	}
	if declared != len(listed) {
		t.Errorf("%d changes declared, %d listed", declared, len(listed))
	}
}
