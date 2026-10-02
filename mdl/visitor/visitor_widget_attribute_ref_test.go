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

// The `$name.Attr` form is read only by the six built-in input builders and
// the combo box (which reads a PARAMETER that way; the builder refuses a data
// view name there). Every other widget that takes `Attribute:` — a dynamic
// text, a data grid column — has no slot to fill and resolved the `$…` string
// to no attribute at all: it executed as "Created page" with the attribute
// silently gone, where it used to be a parse error. It stays one for them.
func TestWidgetAttributeThroughDataViewRefusedOnOtherWidgets(t *testing.T) {
	for _, w := range []string{"dynamictext dt", "image img"} {
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

// A widget outside every data container reads a page or snippet parameter:
// `Attribute: $Param.Attr` (a combo box's association: `$Param.Module.Assoc`)
// and `Visible: $Param.Attr in (…)` — the spellings describe prints for
// Studio Pro's SourceVariable {SnippetParameter|PageParameter: Param}.
func TestParameterBindingsParse(t *testing.T) {
	src := "create snippet M.S (Params: ($Ctx: M.A)) {\n" +
		"  combobox c1 (Attribute: $Ctx.Status)\n" +
		"  combobox c2 (Attribute: $Ctx.M.A_B, DataSource: database M.B, CaptionAttribute: Name)\n" +
		"  image i1 (Visible: $Ctx.Status in (Running, empty))\n" +
		"};"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	ws := prog.Statements[0].(*ast.CreateSnippetStmtV3).Widgets
	if got := ws[0].GetAttribute(); got != "$Ctx.Status" {
		t.Errorf("c1 Attribute = %q, want $Ctx.Status", got)
	}
	if got := ws[1].GetAttribute(); got != "$Ctx.M.A_B" {
		t.Errorf("c2 Attribute = %q, want $Ctx.M.A_B", got)
	}
	vw, ok := ws[2].Properties["VisibleWhen"].(*ast.VisibleWhenV3)
	if !ok || vw.Attribute != "$Ctx.Status" || len(vw.Values) != 2 {
		t.Errorf("i1 VisibleWhen = %+v, want $Ctx.Status in (Running, empty)", ws[2].Properties["VisibleWhen"])
	}
	// The qualified form names an association; on an input it is refused.
	bad := "create snippet M.S (Params: ($Ctx: M.A)) {\n  textbox t1 (Attribute: $Ctx.M.Name)\n};"
	if _, errs := Build(bad); len(errs) == 0 {
		t.Error("textbox (Attribute: $Ctx.M.Name): parsed; want a refusal")
	}
}
