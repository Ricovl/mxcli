// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// mendixlabs/mxcli#1399. Studio Pro 9's "Database" source on a pluggable widget
// is a CustomWidgets$CustomWidgetDatabaseSource: an entity (direct, or reached
// over associations from a parameter or the enclosing object), simple
// Forms$DatabaseConstraint rows instead of an XPath string, and a sort bar.
// parseDataSource did not know the type, and the generic pluggable reader drops
// a datasource it cannot spell, so the FSM template's native snippets described
// their repeaters with no datasource at all. The nested repeaters and actions
// then lost their context object and `check` refused them with CE1571 — which
// the report read as a missing nanoflow argument mapping.
//
// The shapes below are the stored BSON of the FSM template (marketplace 204718,
// Mendix 9.24.2): NativeMobileForm.Snippet_NativeInstanceForm's repeater1 and
// NativeMobile.NativeWorkOrderAssignment_EditRejection's reason list.

// cwRepeaterWithDatabaseSource is a pluggable widget whose one property holds
// ds, with the WidgetValue's own SourceVariable set to sv (nil for none) — on
// 9.x the parameter a database source starts from is stored there, beside the
// datasource, not inside it.
func cwRepeaterWithDatabaseSource(ds map[string]any, sv map[string]any) map[string]any {
	value := map[string]any{"DataSource": ds}
	if sv != nil {
		value["SourceVariable"] = sv
	}
	return map[string]any{
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.native.repeater.Repeater",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": "pt-ds", "PropertyKey": "datasource"},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			map[string]any{"TypePointer": "pt-ds", "Value": value},
		}},
	}
}

func storedCWDatabaseOverAssociation() map[string]any {
	return dsBSON(dsTypeCustomWidgetDatabase,
		"DatabaseConstraints", []any{int32(2)},
		"EntityRef", map[string]any{
			"$Type": "DomainModels$IndirectEntityRef",
			"Steps": []any{int32(2), map[string]any{
				"$Type":             "DomainModels$EntityRefStep",
				"Association":       "NativeMobileForm.NativeInstanceItem_NativeInstanceForm",
				"DestinationEntity": "NativeMobileForm.NativeInstanceItem",
			}},
		},
		"SortBar", map[string]any{"SortItems": []any{int32(2), map[string]any{
			"AttributeRef": map[string]any{"Attribute": "NativeMobileForm.NativeInstanceItem.SequenceNumber"},
			"SortOrder":    "Ascending",
		}}},
	)
}

func storedCWDatabaseWithConstraints() map[string]any {
	return dsBSON(dsTypeCustomWidgetDatabase,
		"DatabaseConstraints", []any{int32(2),
			map[string]any{"$Type": "Forms$DatabaseConstraint", "Attribute": "NativeReason.NativeReason.IsActive", "Operator": "Equals", "Value": "true"},
			map[string]any{"$Type": "Forms$DatabaseConstraint", "Attribute": "NativeReason.NativeReason.ReasonType", "Operator": "NotEquals", "Value": "Unresolved_appointment"},
		},
		"EntityRef", map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "NativeReason.NativeReason"},
		"SortBar", map[string]any{"SortItems": []any{int32(2)}},
	)
}

// The repeater of the report, read through the path every generic pluggable
// widget takes: the snippet parameter is on the WidgetValue.
func TestDescribeCWDatabaseSourceOverAssociationFromSnippetParameter(t *testing.T) {
	w := cwRepeaterWithDatabaseSource(storedCWDatabaseOverAssociation(), map[string]any{
		"$Type": "Forms$PageVariable", "PageParameter": "", "SnippetParameter": "NativeInstanceForm",
		"UseAllPages": false, "Widget": "",
	})
	named := namedCustomWidgetDataSources(w)
	if len(named) != 1 {
		t.Fatalf("the datasource was dropped: %d named datasources", len(named))
	}
	got := dataSourceExpr(nil, named[0].DataSource)
	want := "database from $NativeInstanceForm/NativeMobileForm.NativeInstanceItem_NativeInstanceForm/NativeMobileForm.NativeInstanceItem" +
		" sort by SequenceNumber asc"
	if got != want {
		t.Errorf("describe printed\n  %q\nwant\n  %q", got, want)
	}
	if ctxEntity := dataSourceEntityContext(nil, named[0].DataSource); ctxEntity != "NativeMobileForm.NativeInstanceItem" {
		t.Errorf("the nested widgets' context is %q, want NativeMobileForm.NativeInstanceItem", ctxEntity)
	}
}

// Nested in another data container, no SourceVariable: the steps start from the
// enclosing object, as for a List View's database source over an association.
func TestDescribeCWDatabaseSourceOverAssociationFromCurrentObject(t *testing.T) {
	got := dataSourceExpr(nil, firstObjectPropertyDataSource(cwRepeaterWithDatabaseSource(storedCWDatabaseOverAssociation(), nil)))
	want := "database from $currentObject/NativeMobileForm.NativeInstanceItem_NativeInstanceForm/NativeMobileForm.NativeInstanceItem" +
		" sort by SequenceNumber asc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The constraint rows are the XPath they mean, joined by `and` — the reading
// Mendix's own 9 → 10 conversion gives them when it turns this source into an
// XPath source.
func TestDescribeCWDatabaseSourceConstraints(t *testing.T) {
	got := dataSourceExpr(nil, parseDataSource(storedCWDatabaseWithConstraints()))
	want := "database from NativeReason.NativeReason where [IsActive = true() and ReasonType != 'Unresolved_appointment']"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDatabaseConstraintXPathOperators(t *testing.T) {
	cases := []struct{ op, value, want string }{
		{"Equals", "true", "Flag = true()"},
		{"Equals", "false", "Flag = false()"},
		{"NotEquals", "Open", "Flag != 'Open'"},
		{"LessThan", "5", "Flag < 5"},
		{"LessThanOrEquals", "-2.5", "Flag <= -2.5"},
		{"GreaterThan", "[%CurrentDateTime%]", "Flag > '[%CurrentDateTime%]'"},
		{"GreaterThanOrEquals", "it's", "Flag >= 'it''s'"},
		{"Contains", "ab", "contains(Flag, 'ab')"},
		{"NotContains", "ab", "not(contains(Flag, 'ab'))"},
		{"Equals", "", "Flag = ''"},
	}
	for _, tc := range cases {
		got, ok := databaseConstraintXPath("Mod.E.Flag", tc.op, tc.value)
		if !ok || got != tc.want {
			t.Errorf("%s %q: got %q (ok=%v), want %q", tc.op, tc.value, got, ok, tc.want)
		}
	}
	if _, ok := databaseConstraintXPath("Mod.E.Flag", "Between", "1"); ok {
		t.Error("an operator Mendix does not have was rendered")
	}
}
