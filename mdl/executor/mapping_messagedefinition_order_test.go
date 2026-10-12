// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// Issue #1387: an export mapping over a message definition kept its members in
// SCRIPT order, and mx check (11.12.1) refused it with CE5015 "The order of the
// schema elements under 'Orders/Order' does not match the schema." Studio Pro's
// mapping lists them in the definition's order, which is the only order mxbuild
// accepts — measured: `{ Total = Total, Number = Number }` over a definition
// exposing { Number, Total } is 1 error, the same body in schema order is 0.
// The import twin and both JSON-structure mappings check clean in either order.

func orderMessageDef() *model.MessageDefinitionElement {
	return &model.MessageDefinitionElement{
		Kind: "Entity", Entity: "Shop.Order", ExposedName: "Orders", ExposedItemName: "Order",
		MaxOccurs: -1,
		Children: []*model.MessageDefinitionElement{
			{Kind: "Attribute", Attribute: "Shop.Order.Number", ExposedName: "Number", PrimitiveType: "Integer", MaxOccurs: 1},
			{Kind: "Entity", Entity: "Shop.Line", Association: "Shop.Line_Order", ExposedName: "Lines", ExposedItemName: "Line",
				MaxOccurs: -1,
				Children: []*model.MessageDefinitionElement{
					{Kind: "Attribute", Attribute: "Shop.Line.Sku", ExposedName: "Sku", MaxOccurs: 1},
					{Kind: "Attribute", Attribute: "Shop.Line.Qty", ExposedName: "Qty", PrimitiveType: "Integer", MaxOccurs: 1},
				}},
			// An `alter … add member` lands at the END of its level, after the
			// nested object — so schema order is not "values first".
			{Kind: "Attribute", Attribute: "Shop.Order.Total", ExposedName: "Total", PrimitiveType: "Decimal", MaxOccurs: 1},
		},
	}
}

func exposedNames(elems []*model.ExportMappingElement) []string {
	var out []string
	for _, e := range elems {
		out = append(out, e.ExposedName)
	}
	return out
}

func TestExportMappingFromMessageDefinitionWritesSchemaOrder(t *testing.T) {
	def := &ast.ExportMappingElementDef{
		Entity: "Shop.Order",
		Children: []*ast.ExportMappingElementDef{
			{JsonName: "Total", Attribute: "Total"},
			{Entity: "Shop.Line", Association: "Shop.Line_Order", JsonName: "Lines",
				Children: []*ast.ExportMappingElementDef{
					{JsonName: "Qty", Attribute: "Qty"},
					{JsonName: "Sku", Attribute: "Sku"},
				}},
			{JsonName: "Number", Attribute: "Number"},
		},
	}

	root, err := buildExportMappingFromMessageDefinition("Shop", def, orderMessageDef(), "", "", true, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if got, want := exposedNames(root.Children), []string{"Number", "Lines", "Total"}; !reflect.DeepEqual(got, want) {
		t.Errorf("root members = %v, want schema order %v (CE5015, #1387)", got, want)
	}
	if len(root.Children) == 3 {
		if got, want := exposedNames(root.Children[1].Children), []string{"Sku", "Qty"}; !reflect.DeepEqual(got, want) {
			t.Errorf("nested members = %v, want schema order %v (CE5015, #1387)", got, want)
		}
	}
}

// A subset in schema order must come through untouched — the sort places
// members, it does not invent the ones the script left out.
func TestExportMappingFromMessageDefinitionKeepsSubset(t *testing.T) {
	def := &ast.ExportMappingElementDef{
		Entity: "Shop.Order",
		Children: []*ast.ExportMappingElementDef{
			{JsonName: "Total", Attribute: "Total"},
			{JsonName: "Number", Attribute: "Number"},
		},
	}
	root, err := buildExportMappingFromMessageDefinition("Shop", def, orderMessageDef(), "", "", true, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, want := exposedNames(root.Children), []string{"Number", "Total"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
}
