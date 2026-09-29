// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#826: an input's attribute read through a named data view,
// `Attribute: $dataView1.FullName` — the form describe prints for Studio Pro's
// widget-scoped SourceVariable. It used to be a parse error ("property
// Attribute takes a plain value, not an expression").
func TestWidgetAttributeThroughDataViewParses(t *testing.T) {
	for _, tc := range []struct{ attr, want string }{
		{"$dataView1.FullName", "$dataView1.FullName"},
		{`$dataView1."Type"`, "$dataView1.Type"},
		{"Name", "Name"},             // control
		{"Assoc/Name", "Assoc/Name"}, // control
	} {
		src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($A: M.A)) {\n" +
			"  dataview dataView1 (DataSource: $A) { textbox tb (Attribute: " + tc.attr + ") }\n};"
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Errorf("Attribute: %s: parse error %v", tc.attr, errs[0])
			continue
		}
		stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
		tb := stmt.Widgets[0].Children[0]
		if got := tb.GetAttribute(); got != tc.want {
			t.Errorf("Attribute: %s: got %q, want %q", tc.attr, got, tc.want)
		}
	}
}

// The `$name.Attr` form is read only by the six built-in input builders. Every
// other widget that takes `Attribute:` — a combo box, a dynamic text, a data
// grid column — has no Widget slot to fill and resolved the `$…` string to no
// attribute at all: `combobox cb (Attribute: $dataView1.Name)` executed as
// "Created page" with the attribute silently gone, where it used to be a parse
// error. It stays one for them.
func TestWidgetAttributeThroughDataViewRefusedOnOtherWidgets(t *testing.T) {
	for _, w := range []string{"combobox cb", "dynamictext dt", "image img"} {
		src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($A: M.A)) {\n" +
			"  dataview dataView1 (DataSource: $A) { " + w + " (Attribute: $dataView1.Name) }\n};"
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("%s (Attribute: $dataView1.Name): parsed; want a refusal", w)
		}
	}
	// Control: every input the builder resolves the form for still parses.
	for _, w := range []string{"textbox", "textarea", "checkbox", "datepicker", "radiobuttons", "dropdown"} {
		src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ($A: M.A)) {\n" +
			"  dataview dataView1 (DataSource: $A) { " + w + " w1 (Attribute: $dataView1.Name) }\n};"
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("%s (Attribute: $dataView1.Name): %v", w, errs[0])
		}
	}
}
