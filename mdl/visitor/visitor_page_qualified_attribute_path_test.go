// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `Attribute: Module.Assoc/Attr` — an association hop written module-qualified,
// the way XPath and data view sources name it — is an attribute path, exactly
// like the bare `Assoc/Attr`. attributePathV3 accepted only plain identifiers
// between slashes, so the qualified form fell through to the generic
// `keyword: expression` alternative, parsed as the division `M.Assoc / Attr`,
// and check refused it as MDL-WIDGET34 + MDL-WIDGET42 ("cannot hold the
// expression").
func TestQualifiedAssociationAttributePathParses(t *testing.T) {
	cases := []struct {
		name, prop, want string
	}{
		{"datagrid column", "column c1 (Attribute: FieldService.ServiceRequest_LithoSystem/SerialNumber, Caption: 'S')",
			"FieldService.ServiceRequest_LithoSystem/SerialNumber"},
		{"quoted segments", `column c1 (Attribute: "FieldService"."ServiceRequest_LithoSystem"/"SerialNumber")`,
			"FieldService.ServiceRequest_LithoSystem/SerialNumber"},
		{"two hops", "column c1 (Attribute: M.A_B/M.B_C/Name)", "M.A_B/M.B_C/Name"},
		{"bare unchanged", "column c1 (Attribute: ServiceRequest_LithoSystem/SerialNumber)",
			"ServiceRequest_LithoSystem/SerialNumber"},
		{"plain name unchanged", "column c1 (Attribute: SerialNumber)", "SerialNumber"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create page M.P (Title: 'x', Layout: A.L) { datagrid dg (DataSource: database M.E) { " + tc.prop + " } };"
			prog, errs := Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			grid := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
			if len(grid.Children) != 1 {
				t.Fatalf("grid children = %d, want 1", len(grid.Children))
			}
			got, ok := grid.Children[0].Properties["Attribute"].(string)
			if !ok {
				t.Fatalf("Attribute = %#v (%T), want the attribute path string %q",
					grid.Children[0].Properties["Attribute"], grid.Children[0].Properties["Attribute"], tc.want)
			}
			if got != tc.want {
				t.Errorf("Attribute = %q, want %q", got, tc.want)
			}
		})
	}
}

// The qualified hop works on an input widget too, and the final segment stays
// a bare name: `Module.Entity.Attr` with no slash is not an association path.
func TestQualifiedAssociationAttributePathOnTextBox(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) { dataview dv (DataSource: $P) { textbox tb (Attribute: M.Order_Customer/Name) } };"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	tb := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children[0]
	if got := tb.Properties["Attribute"]; got != "M.Order_Customer/Name" {
		t.Errorf("Attribute = %#v, want %q", got, "M.Order_Customer/Name")
	}
}
