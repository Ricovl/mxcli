// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFormsWidgetsWithoutWriterAreUnbuilt keeps formsWidgetsWithoutWriter — the
// one answer check and exec share about which keywords the page builder cannot
// write (ako/mxcli#563) — in step with the builder. Each entry must be a widget
// keyword the grammar accepts, and must NOT be a case of buildWidgetV3's
// dispatch: a keyword that gains a builder has to leave the set, or check goes
// on refusing a widget exec can now write.
func TestFormsWidgetsWithoutWriterAreUnbuilt(t *testing.T) {
	cases := builderSwitchCases(t)
	if !cases["textbox"] || !cases["dataview"] {
		t.Fatalf("could not read buildWidgetV3's dispatch (got %d cases) — the guard would pass vacuously", len(cases))
	}
	grammar, err := os.ReadFile(filepath.Join("..", "grammar", "domains", "MDLPage.g4"))
	if err != nil {
		t.Fatal(err)
	}
	rule := string(grammar)
	if i := strings.Index(rule, "\nwidgetTypeV3"); i >= 0 {
		rule = rule[i:]
		if j := strings.Index(rule, ";"); j >= 0 {
			rule = rule[:j]
		}
	}
	for kw := range formsWidgetsWithoutWriter {
		if cases[kw] {
			t.Errorf("%q has a case in buildWidgetV3 now — drop it from formsWidgetsWithoutWriter", kw)
		}
		if !strings.Contains(rule, strings.ToUpper(kw)) {
			t.Errorf("%q is not a widgetTypeV3 keyword — formsWidgetsWithoutWriter lists only what parses", kw)
		}
	}
}

// builderSwitchCases returns the string cases of the `switch strings.ToLower(
// w.Type)` in buildWidgetV3.
func builderSwitchCases(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "cmd_pages_builder_v3.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "buildWidgetV3" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						out[s] = true
					}
				}
			}
			return true
		})
	}
	return out
}
