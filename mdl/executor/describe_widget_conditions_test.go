// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// R5 (ako/mxcli#753): describe page writes a conditional Visible / Editable as
// the bare expression it stores; one that would read back as a plain value is
// bracketed, as before.
func TestDescribePage_WidgetConditionIsBare(t *testing.T) {
	for _, tc := range []struct{ key, expr, want string }{
		{"Visible", "$currentObject/Active", "Visible: $currentObject/Active"},
		{"Editable", "$currentObject/Status = 'Open'", "Editable: $currentObject/Status = 'Open'"},
		{"Visible", "true", "Visible: [true]"},
	} {
		if got := widgetConditionMDL(tc.key, tc.expr); got != tc.want {
			t.Errorf("widgetConditionMDL(%q, %q) = %q, want %q", tc.key, tc.expr, got, tc.want)
		}
	}
	props := appendAppearanceProps(nil, rawWidget{VisibleIf: "$currentObject/A and $currentObject/B", EditableIf: "$currentObject/C"})
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { textbox t (Attribute: Name, " +
		strings.Join(props, ", ") + ") } };"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("does not re-parse: %v\n%s", errs, src)
	}
	if len(prog.Deprecations) != 0 {
		t.Errorf("deprecations %v", prog.Deprecations)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children[0]
	if w.Properties["VisibleIf"] != "$currentObject/A and $currentObject/B" || w.Properties["EditableIf"] != "$currentObject/C" {
		t.Errorf("re-parsed as %#v", w.Properties)
	}
}
