// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1584: a quoted attribute segment in DynamicClasses was
// stored with its quotes — `$currentObject/"Code"` — while ContentParams and
// Visible on the same page stored `$currentObject/Code`. check passed, mxbuild
// reported CE0117 "Error(s) in expression." at the widget. Every widget slot
// that stores expression text strips identifier quotes, as the other
// expression slots do (#827, #1481).

func TestWidgetExpressionSlots_StripIdentifierQuotes(t *testing.T) {
	const quoted = `if $currentObject/"Code" = empty then 'x' else ''`
	const want = `if $currentObject/Code = empty then 'x' else ''`

	t.Run("DynamicClasses and DynamicCellClass", func(t *testing.T) {
		prog := mustBuild(t, dynamicClassPage("mdl 1;\n", quoted))
		cls, cell := dynamicClassValues(t, prog)
		if cls != want || cell != want {
			t.Errorf("stored %#v / %#v, want %q", cls, cell, want)
		}
	})

	t.Run("mdl 0 quoted spelling", func(t *testing.T) {
		legacy := `'if $currentObject/"Code" = empty then ''x'' else '''''`
		prog := mustBuild(t, dynamicClassPage("", legacy))
		cls, cell := dynamicClassValues(t, prog)
		if cls != want || cell != want {
			t.Errorf("stored %#v / %#v, want %q", cls, cell, want)
		}
		// The rewrite to the bare form still applies: the bare form stores the same.
		for _, n := range quotedExprNotes(prog) {
			if n.Fix == nil {
				t.Errorf("note %+v offers no rewrite", n)
			}
		}
	})

	t.Run("alter page set", func(t *testing.T) {
		prog := mustBuild(t, "mdl 1;\nalter page M.P { set DynamicClasses = "+quoted+" on c1 };")
		op := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
		if got := op.Properties["DynamicClasses"]; got != want {
			t.Errorf("stored %#v, want %q", got, want)
		}
	})

	t.Run("pluggable expression property", func(t *testing.T) {
		prog := mustBuild(t, `mdl 1;
create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.custom.badge.Badge' b1 (barColor: if $currentObject/"Code" = empty then 'x' else '', lineColor: $currentObject/"Code")
};`)
		w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
		expr, ok := w.Properties["barColor"].(*ast.WidgetExpressionV3)
		if !ok || expr.Text != want {
			t.Errorf("barColor = %#v, want expression %q", w.Properties["barColor"], want)
		}
		if src, _ := w.ExpressionSource("lineColor"); src != "$currentObject/Code" {
			t.Errorf("lineColor source = %q, want %q", src, "$currentObject/Code")
		}
	})
}
