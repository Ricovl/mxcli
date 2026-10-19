// SPDX-License-Identifier: Apache-2.0

// A data container bound over an association reported the *association* as its
// context entity — `-- Context: $currentObject (Shop.Order_Customer)` instead of
// `(Shop.Customer)` — because the datasource reference was used verbatim. That is
// right for a database source, where the reference is the entity, and wrong for
// a microflow source (its flow) and for an association source (its association).
// The flow case was fixed first; these tests cover the association case.
//
// Storage carries the answer: every step of an association source's EntityRef
// stores the DestinationEntity it arrives at, in either direction.
package executor

import (
	"strings"
	"testing"
)

func assocStep(assoc, dest string) map[string]any {
	return map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": assoc, "DestinationEntity": dest}
}

func indirectRef(steps ...map[string]any) map[string]any {
	items := make([]any, len(steps))
	for i, s := range steps {
		items[i] = s
	}
	return map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": items}
}

func pageVariable(param string) map[string]any {
	return map[string]any{"$Type": "Forms$PageVariable", "PageParameter": param}
}

// assocDataSource is a Forms$AssociationSource over steps from $currentObject.
func assocDataSource(steps ...map[string]any) map[string]any {
	return map[string]any{"$Type": "Forms$AssociationSource", "EntityRef": indirectRef(steps...)}
}

// outerOrderView wraps inner in a data view over page parameter $Order, the way
// the field report nests it.
func outerOrderView(inner map[string]any) map[string]any {
	return map[string]any{
		"$Type": "Forms$DataView",
		"Name":  "dvOuter",
		"DataSource": map[string]any{
			"$Type":          "Forms$DataViewSource",
			"EntityRef":      map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "Shop.Order"},
			"SourceVariable": pageVariable("Order"),
		},
		"Widgets": []any{int32(2), inner},
	}
}

// A context comment is written only above a container's body, so each nested
// container carries one child.
func containerChild() map[string]any {
	return map[string]any{"$Type": "Forms$TextBox", "Name": "tbInner"}
}

func nestedDataView(ds map[string]any) map[string]any {
	return map[string]any{"$Type": "Forms$DataView", "Name": "dvInner", "DataSource": ds, "Widgets": []any{int32(2), containerChild()}}
}

func nestedListView(ds map[string]any) map[string]any {
	return map[string]any{"$Type": "Forms$ListView", "Name": "lvInner", "DataSource": ds, "Widgets": []any{int32(2), containerChild()}}
}

// A data view over a reference reaches the association's other end, and a list
// view over a reference set reaches its elements — in both directions.
func TestDescribe_AssociationSourceContextIsTheEntityReached(t *testing.T) {
	tests := []struct {
		name      string
		inner     map[string]any
		wantLine  string
		wantExpr  string
		notWanted string
	}{
		{
			name:      "reference, owner to child",
			inner:     nestedDataView(map[string]any{"$Type": "Forms$DataViewSource", "EntityRef": indirectRef(assocStep("Shop.Order_Customer", "Shop.Customer")), "SourceVariable": pageVariable("")}),
			wantLine:  "-- Context: $currentObject (Shop.Customer)\n",
			wantExpr:  "DataSource: $currentObject/Shop.Order_Customer",
			notWanted: "(Shop.Order_Customer)",
		},
		{
			name:      "reference seen from the child's side",
			inner:     nestedListView(assocDataSource(assocStep("Shop.Order_Customer", "Shop.Order"))),
			wantLine:  "-- Context: $currentObject (Shop.Order), $lvInner (selection)\n",
			wantExpr:  "DataSource: $currentObject/Shop.Order_Customer",
			notWanted: "(Shop.Order_Customer)",
		},
		{
			name:      "reference set",
			inner:     nestedListView(assocDataSource(assocStep("Shop.Order_Line", "Shop.OrderLine"))),
			wantLine:  "-- Context: $currentObject (Shop.OrderLine), $lvInner (selection)\n",
			wantExpr:  "DataSource: $currentObject/Shop.Order_Line",
			notWanted: "(Shop.Order_Line)",
		},
		{
			// The stored destination can be a specialization of the association's
			// own end, and that — not the end — is the entity in scope.
			name:      "stored destination is a specialization of the end",
			inner:     nestedListView(assocDataSource(assocStep("Shop.Order_Line", "Shop.SpecialLine"))),
			wantLine:  "-- Context: $currentObject (Shop.SpecialLine), $lvInner (selection)\n",
			wantExpr:  "DataSource: $currentObject/Shop.Order_Line",
			notWanted: "(Shop.Order_Line)",
		},
		{
			name: "two hops: the last destination",
			inner: nestedListView(assocDataSource(
				assocStep("Shop.Order_Customer", "Shop.Customer"),
				assocStep("Shop.Address_Customer", "Shop.Address"))),
			wantLine:  "-- Context: $currentObject (Shop.Address), $lvInner (selection)\n",
			wantExpr:  "DataSource: $currentObject/Shop.Order_Customer/Shop.Customer/Shop.Address_Customer/Shop.Address",
			notWanted: "(Shop.Order_Customer",
		},
		{
			name: "over a page parameter instead of the context object",
			inner: nestedDataView(map[string]any{
				"$Type":          "Forms$DataViewSource",
				"EntityRef":      indirectRef(assocStep("Shop.Order_Customer", "Shop.Customer")),
				"SourceVariable": pageVariable("Order"),
			}),
			wantLine:  "-- Context: $currentObject (Shop.Customer)\n",
			wantExpr:  "DataSource: $Order/Shop.Order_Customer",
			notWanted: "(Shop.Order_Customer)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := describeWidget(t, outerOrderView(tc.inner))
			if !strings.Contains(got, tc.wantLine) {
				t.Errorf("describe output lacks %q:\n%s", tc.wantLine, got)
			}
			if strings.Contains(got, tc.notWanted) {
				t.Errorf("describe output still names the association as the context %q:\n%s", tc.notWanted, got)
			}
			// Only the context comment changes: the datasource text is what
			// exec reads back.
			if !strings.Contains(got, tc.wantExpr) {
				t.Errorf("describe output lacks the unchanged datasource %q:\n%s", tc.wantExpr, got)
			}
		})
	}
}

// The pluggable Gallery / Data grid 2 / chart datasource reads through the same
// parser (#941): an XPath source with no entity and the association hops in
// EntityRef.
func TestParseDataSource_PluggableAssociationSourceContextIsTheEntityReached(t *testing.T) {
	ds := parseDataSource(map[string]any{
		"$Type":     "CustomWidgets$CustomWidgetXPathSource",
		"EntityRef": indirectRef(assocStep("Shop.Order_Line", "Shop.OrderLine")),
	})
	if ds == nil || ds.Type != "association" {
		t.Fatalf("parseDataSource = %+v, want an association source", ds)
	}
	if got := dataSourceEntityContext(nil, ds); got != "Shop.OrderLine" {
		t.Errorf("dataSourceEntityContext = %q, want Shop.OrderLine", got)
	}
	if got := dataSourceExpr(nil, ds); got != "$currentObject/Shop.Order_Line" {
		t.Errorf("dataSourceExpr = %q, want the unchanged $currentObject/Shop.Order_Line", got)
	}
}

// A source whose destination is not stored keeps the reference: no worse than
// before, as the flow fix does for a flow it cannot resolve.
func TestDataSourceEntityContext_AssociationWithoutStoredDestinationKeepsTheReference(t *testing.T) {
	ds := parseDataSource(assocDataSource(assocStep("Shop.Order_Customer", "")))
	if ds == nil || ds.Type != "association" {
		t.Fatalf("parseDataSource = %+v, want an association source", ds)
	}
	if got := dataSourceEntityContext(nil, ds); got != "Shop.Order_Customer" {
		t.Errorf("dataSourceEntityContext = %q, want the reference kept", got)
	}

	// Two hops of which one has no stored destination: the path prints the
	// associations alone, and so does the fallback.
	ds = parseDataSource(assocDataSource(assocStep("Shop.A_B", "Shop.B"), assocStep("Shop.B_C", "")))
	if got := dataSourceEntityContext(nil, ds); got != "Shop.A_B/Shop.B_C" {
		t.Errorf("dataSourceEntityContext = %q, want the reference kept", got)
	}
}

// Controls: a database source keeps naming its own entity, and so does a
// datasource reached over associations that is still a database retrieve.
func TestDescribe_DatabaseSourceContextUnchanged(t *testing.T) {
	got := describeWidget(t, outerOrderView(nestedListView(map[string]any{
		"$Type":     "Forms$ListViewXPathSource",
		"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "Shop.OrderLine"},
	})))
	if want := "-- Context: $currentObject (Shop.OrderLine), $lvInner (selection)\n"; !strings.Contains(got, want) {
		t.Errorf("describe output lacks %q:\n%s", want, got)
	}

	got = describeWidget(t, outerOrderView(nestedListView(map[string]any{
		"$Type":     "Forms$ListViewXPathSource",
		"EntityRef": indirectRef(assocStep("Shop.Order_Line", "Shop.OrderLine")),
	})))
	if want := "-- Context: $currentObject (Shop.OrderLine), $lvInner (selection)\n"; !strings.Contains(got, want) {
		t.Errorf("describe output lacks %q:\n%s", want, got)
	}
}
