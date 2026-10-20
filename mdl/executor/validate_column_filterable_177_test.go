// SPDX-License-Identifier: Apache-2.0

// `column (Attribute: Name, Filterable: true)` (mendixlabs/mxcli#177).
//
// Reported: "there is no way to set `Filterable: true` on individual columns.
// The property is silently ignored." A column's filter is not a property: it
// is the `filter` widgets slot, written as a filter widget inside the column's
// braces. check now refuses the key, but the message listed the column's
// properties (with a project) or suggested `FilterType` (without one) — neither
// says where a column filter goes.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestColumnFilterable_PointsAtTheFilterSlot(t *testing.T) {
	col := &ast.WidgetV3{Type: "column", Name: "c1", Properties: map[string]any{"Attribute": "Name", "Filterable": true}}

	mapping := &ObjectListMapping{
		MDLContainer: "COLUMN",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "attribute", Operation: "attribute"},
			{PropertyKey: "header", Operation: "texttemplate", MdlAliases: []string{"Caption"}},
		},
		ItemSlots: []ItemSlotMapping{
			{PropertyKey: "content", Operation: "widgets"},
			{PropertyKey: "filter", Operation: "widgets", AcceptedChildTypes: []string{"textfilter", "numberfilter"}},
		},
	}
	withProject := validateObjectListItemUnknownProps(col, mapping, "page X")

	grid := &ast.WidgetV3{Type: "datagrid", Name: "dg"}
	schema := embeddedSchemaFor(col, grid)
	if schema == nil {
		t.Fatal("no embedded Data grid 2 column schema")
	}
	withoutProject := validateStaticWidgetUnknownPropsIn(col, "page X", schema, false)

	for name, vs := range map[string]int{"with a project": len(withProject), "without one": len(withoutProject)} {
		if vs != 1 {
			t.Fatalf("%s: want one violation for Filterable, got %d", name, vs)
		}
	}
	for name, msg := range map[string]string{
		"with a project": withProject[0].Message,
		"without one":    withoutProject[0].Message,
	} {
		for _, want := range []string{"`filter`", "textfilter"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: message should point at the filter slot (%s): %s", name, want, msg)
			}
		}
		if strings.Contains(msg, "FilterType") {
			t.Errorf("%s: suggests FilterType, which a column does not have: %s", name, msg)
		}
	}
}

// Control: a plain typo still gets the nearest-key suggestion.
func TestColumnUnknownKey_TypoStillSuggests(t *testing.T) {
	col := &ast.WidgetV3{Type: "column", Name: "c1", Properties: map[string]any{"Sortabel": true}}
	schema := embeddedSchemaFor(col, &ast.WidgetV3{Type: "datagrid", Name: "dg"})
	vs := validateStaticWidgetUnknownPropsIn(col, "page X", schema, false)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "did you mean `sortable`") {
		t.Errorf("want a sortable suggestion, got %+v", vs)
	}
}
