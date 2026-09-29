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
