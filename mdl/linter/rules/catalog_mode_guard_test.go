// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// fullOnlyContext names the LintContext methods and row fields backed by tables
// or columns only REFRESH CATALOG FULL writes. Under the default fast build
// they come back empty, so a rule reading one without declaring
// RequiredCatalogMode() >= CatalogFull reports nothing and says nothing —
// MPR005, MPR006 and MPR012 were all silent that way under `mxcli lint`.
// The Starlark side of the same list is derived empirically in
// mdl/linter/starlark_catalog_mode_guard_test.go; keep the two in step.
var fullOnlyContext = []string{
	"Widgets", "XPathExpressions", "ActivitiesFor", "Permissions", "PermissionsFor",
	"FindReferences", "WidgetCount",
}

func TestRulesReadingFullOnlyDataDeclareFullCatalog(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	uses := map[string][]string{} // receiver type -> full-only members it reads
	declares := map[string]bool{} // receiver type -> has RequiredCatalogMode
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				recv := receiverName(fn.Recv.List[0].Type)
				if fn.Name.Name == "RequiredCatalogMode" {
					declares[recv] = true
				}
				ast.Inspect(fn, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					for _, m := range fullOnlyContext {
						if sel.Sel.Name == m {
							uses[recv] = append(uses[recv], m)
						}
					}
					return true
				})
			}
		}
	}
	if len(uses) == 0 {
		t.Fatal("no rule reads full-only data; the scan found nothing to check")
	}
	for recv, ms := range uses {
		if !declares[recv] {
			t.Errorf("%s reads %v (full-catalog data) but does not implement "+
				"RequiredCatalogMode() — under the default fast catalog it silently "+
				"finds nothing", recv, ms)
		}
	}
}

func receiverName(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
