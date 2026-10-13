// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1399. A 9.x "Database" source on a pluggable widget is a
// CustomWidgets$CustomWidgetDatabaseSource, and Studio Pro 9.24.2 stores exactly
// five keys on it — measured on all ten in the FSM template (marketplace 204718):
// $ID, $Type, DatabaseConstraints, EntityRef, SortBar. No XPathConstraint, no
// ForceFullObjects (Forms$DataSource gains it in 10.8.0) and no SourceVariable
// (10.0.0): the parameter the path starts from is on the WidgetValue.
func TestCustomWidgetDatabaseSourceToGen_StudioProShape(t *testing.T) {
	el, err := customWidgetDataSourceToGen(&pages.DatabaseSource{
		EntityName: "NativeMobileForm.NativeInstanceItem",
		EntitySteps: []pages.AttributeRefStep{{
			Association:       "NativeMobileForm.NativeInstanceItem_NativeInstanceForm",
			DestinationEntity: "NativeMobileForm.NativeInstanceItem",
		}},
		ContextVariable:    "NativeInstanceForm",
		IsSnippetParameter: true,
		Sorting: []*pages.GridSort{{
			AttributePath: "NativeMobileForm.NativeInstanceItem.SequenceNumber", Direction: "Ascending",
		}},
		UseConstraints: true,
		Constraints: []pages.DatabaseConstraint{
			{Attribute: "NativeMobileForm.NativeInstanceItem.IsMandatory", Operator: "Equals", Value: "true"},
		},
	})
	if err != nil {
		t.Fatalf("customWidgetDataSourceToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := lookup(d, "$Type"); got != "CustomWidgets$CustomWidgetDatabaseSource" {
		t.Fatalf("$Type = %v, want CustomWidgets$CustomWidgetDatabaseSource", got)
	}
	keys := dKeys(d)
	sort.Strings(keys)
	if want := []string{"$ID", "$Type", "DatabaseConstraints", "EntityRef", "SortBar"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}

	ref, _ := lookup(d, "EntityRef").(bson.D)
	if lookup(ref, "$Type") != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef = %v, want an IndirectEntityRef", ref)
	}
	rows, _ := lookup(d, "DatabaseConstraints").(bson.A)
	if len(rows) != 2 || rows[0] != int32(2) {
		t.Fatalf("DatabaseConstraints = %v, want marker 2 and one row", rows)
	}
	row, _ := rows[1].(bson.D)
	for k, want := range map[string]any{
		"$Type": "Forms$DatabaseConstraint", "Attribute": "NativeMobileForm.NativeInstanceItem.IsMandatory",
		"Operator": "Equals", "Value": "true",
	} {
		if got := lookup(row, k); got != want {
			t.Errorf("constraint %s = %v, want %v", k, got, want)
		}
	}
	bar, _ := lookup(d, "SortBar").(bson.D)
	if items, _ := lookup(bar, "SortItems").(bson.A); len(items) != 2 {
		t.Errorf("SortBar.SortItems = %v, want one sort item", items)
	}
}

// An empty constraint list is still stored, with marker 2, as in every one of
// the FSM template's sources.
func TestCustomWidgetDatabaseSourceToGen_EmptyConstraints(t *testing.T) {
	el, err := customWidgetDataSourceToGen(&pages.DatabaseSource{EntityName: "M.E", UseConstraints: true})
	if err != nil {
		t.Fatalf("customWidgetDataSourceToGen: %v", err)
	}
	raw, _ := (&codec.Encoder{}).Encode(el)
	var d bson.D
	_ = bson.Unmarshal(raw, &d)
	if rows, ok := lookup(d, "DatabaseConstraints").(bson.A); !ok || len(rows) != 1 || rows[0] != int32(2) {
		t.Errorf("DatabaseConstraints = %#v, want [2]", lookup(d, "DatabaseConstraints"))
	}
	if ref, _ := lookup(d, "EntityRef").(bson.D); lookup(ref, "$Type") != "DomainModels$DirectEntityRef" {
		t.Errorf("EntityRef = %v, want a DirectEntityRef", ref)
	}
}
