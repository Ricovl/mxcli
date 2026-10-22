// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mendixlabs/mxcli#1519, item 3b:
//
//	set (ColumnWidth: manual, Size: 220) on dg column(Name)
//	set (ColumnWidth: autoFill) on dg column(Name)
//
// stored width=autoFill with size=220, and mx check answered CE0463 (11.6.6 and
// 11.15.0). Data grid 2 hides `size` unless `width` is "manual" — rules as the
// editorConfig states them, lifted per column item.

const (
	idWidth    = "aaaaaaaa-0000-0000-0000-000000000001"
	idSize     = "aaaaaaaa-0000-0000-0000-000000000002"
	idMinWidth = "aaaaaaaa-0000-0000-0000-000000000003"
	idMinLimit = "aaaaaaaa-0000-0000-0000-000000000004"
)

// widthColumn is a column as `set (ColumnWidth: manual, Size: 220)` left it,
// with width already switched to newWidth by the setter.
func widthColumn(newWidth, minWidth string) (bson.D, map[string]string, map[string]string) {
	prop := func(id, v string) bson.D {
		return bson.D{
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(id)},
			{Key: "Value", Value: bson.D{{Key: "Expression", Value: ""}, {Key: "PrimitiveValue", Value: v}}},
		}
	}
	col := bson.D{{Key: "Properties", Value: bson.A{
		prop(idWidth, newWidth), prop(idSize, "220"), prop(idMinWidth, minWidth), prop(idMinLimit, "250"),
	}}}
	keys := map[string]string{idWidth: "width", idSize: "size", idMinWidth: "minWidth", idMinLimit: "minWidthLimit"}
	kinds := map[string]string{idWidth: "Enumeration", idSize: "Integer", idMinWidth: "Enumeration", idMinLimit: "Integer"}
	return col, keys, kinds
}

var columnDefaults = map[string]string{"width": "autoFill", "size": "1", "minWidth": "auto", "minWidthLimit": "100"}

// The three Data grid 2 3.x column-width rules, as the extractor lifts them.
func widthRules() []types.WidgetVisibilityRule {
	item := func(key, cond, op, val string) types.WidgetVisibilityRule {
		return types.WidgetVisibilityRule{PropertyKey: key, ListPropertyKey: "columns",
			HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: cond, Operator: op, Value: val, Scope: types.ConditionScopeItem}}
	}
	return []types.WidgetVisibilityRule{
		item("size", "width", "ne", "manual"),
		item("minWidth", "width", "ne", "autoFit"),
		item("minWidthLimit", "minWidth", "ne", "manual"),
	}
}

func TestResetHiddenColumnProps1519_SizeGoesBackToDefault(t *testing.T) {
	col, keys, kinds := widthColumn("autoFill", "auto")
	got, err := resetHiddenItemProps(col, keys, kinds, columnDefaults, nil, "columns", widthRules(), map[string]bool{"width": true}, nil)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if v := fieldOf(t, col, idSize, "PrimitiveValue"); v != "1" {
		t.Errorf("size = %v after switching width to autoFill, want the default 1 — 220 is CE0463", v)
	}
	if strings.Join(got, ",") != "minWidthLimit,size" {
		t.Errorf("reset keys = %v, want [minWidthLimit size]", got)
	}
}

// A reset that hides a further property resets that one too: autoFill hides
// minWidth, and minWidth back at "auto" hides minWidthLimit.
func TestResetHiddenColumnProps1519_CascadesThroughDependentRules(t *testing.T) {
	col, keys, kinds := widthColumn("autoFill", "manual")
	if _, err := resetHiddenItemProps(col, keys, kinds, columnDefaults, nil, "columns", widthRules(), nil, nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if v := fieldOf(t, col, idMinWidth, "PrimitiveValue"); v != "auto" {
		t.Errorf("minWidth = %v, want auto (hidden unless width is autoFit)", v)
	}
	if v := fieldOf(t, col, idMinLimit, "PrimitiveValue"); v != "100" {
		t.Errorf("minWidthLimit = %v, want 100 (hidden once minWidth is back at auto)", v)
	}
}

// CONTROL: under "manual" size is visible and keeps the author's value.
func TestResetHiddenColumnProps1519_VisibleSizeIsKept(t *testing.T) {
	col, keys, kinds := widthColumn("manual", "auto")
	if _, err := resetHiddenItemProps(col, keys, kinds, columnDefaults, nil, "columns", widthRules(), nil, nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if v := fieldOf(t, col, idSize, "PrimitiveValue"); v != "220" {
		t.Errorf("size = %v under width manual, want 220 untouched", v)
	}
}

// A hidden property the SAME statement names is not silently overwritten:
// `set (ColumnWidth: autoFill, Size: 220)` is refused, the way MDL-WIDGET10
// refuses it on create. Its default is still accepted.
func TestResetHiddenColumnProps1519_NamedHiddenPropertyIsRefused(t *testing.T) {
	col, keys, kinds := widthColumn("autoFill", "auto")
	_, err := resetHiddenItemProps(col, keys, kinds, columnDefaults, nil, "columns", widthRules(), map[string]bool{"size": true}, nil)
	if err == nil || !strings.Contains(err.Error(), "CE0463") || !strings.Contains(err.Error(), `"size"`) {
		t.Fatalf("want a refusal naming size and CE0463, got %v", err)
	}
}

// The defaults come from the stored Type, not from a table in mxcli.
func TestColumnPropDefaults1519_ReadFromStoredType(t *testing.T) {
	pt := func(key, def string) bson.D {
		return bson.D{{Key: "PropertyKey", Value: key}, {Key: "ValueType", Value: bson.D{{Key: "DefaultValue", Value: def}}}}
	}
	grid := bson.D{{Key: "Type", Value: bson.D{{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{
		int32(2),
		bson.D{{Key: "PropertyKey", Value: "columns"}, {Key: "ValueType", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2), pt("width", "autoFill"), pt("size", "1")}}}},
		}}},
	}}}}}}}
	got := columnPropDefaults(grid, "columns")
	if got["size"] != "1" || got["width"] != "autoFill" {
		t.Errorf("defaults = %v, want size=1 width=autoFill", got)
	}
}
