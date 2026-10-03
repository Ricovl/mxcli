// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// Tests for the widget tree, appearance and action columns
// (mendixlabs/mxcli#1268). The walker used to flatten a page into a list with
// no parent, so a lint rule could not ask how deeply a widget is nested, and it
// read neither the widget's Appearance nor its action.

func wid(id, name, typ string, extra map[string]any) map[string]any {
	m := map[string]any{"$ID": id, "Name": name, "$Type": typ}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func bsonList(items ...any) []any { return append([]any{int32(3)}, items...) }

// assertTree checks each named widget's parent (by name, "" for the root) and depth.
func assertTree(t *testing.T, rows []rawWidgetInfo, want map[string]struct {
	parent string
	depth  int
}) {
	t.Helper()
	nameByID := map[string]string{}
	for _, r := range rows {
		nameByID[r.ID] = r.Name
	}
	for name, w := range want {
		got := widgetByName(rows, name)
		if got == nil {
			t.Errorf("widget %q not indexed", name)
			continue
		}
		if p := nameByID[got.ParentID]; p != w.parent || (w.parent == "" && got.ParentID != "") {
			t.Errorf("%s: parent = %q (id %q), want %q", name, p, got.ParentID, w.parent)
		}
		if got.Depth != w.depth {
			t.Errorf("%s: depth = %d, want %d", name, got.Depth, w.depth)
		}
	}
}

type tp = struct {
	parent string
	depth  int
}

// A skipped wrapper is transparent: its child is parented to the wrapper's own
// parent at the wrapper's depth, not left dangling on an ID absent from the table.
func TestWidgetTree_ParentThroughSkippedWrapper(t *testing.T) {
	page := wid("dv", "dataView1", "Forms$DataView", map[string]any{
		"Widgets": bsonList(
			wid("wrap", "conditionalVisibilityWidget1", "Forms$DivContainer", map[string]any{
				"Widgets": bsonList(wid("tb", "textBox1", "Forms$TextBox", nil)),
			}),
		),
	})
	rows := extractWidgetsRecursive(page)
	if widgetByName(rows, "conditionalVisibilityWidget1") != nil {
		t.Fatal("synthetic wrapper must stay unindexed")
	}
	assertTree(t, rows, map[string]tp{
		"dataView1": {"", 0},
		"textBox1":  {"dataView1", 1},
	})
}

// Layout grid rows and columns, and tab pages, are not widgets: a widget in a
// column is the grid's child, one in a tab page the tab container's.
func TestWidgetTree_LayoutGridAndTabContainer(t *testing.T) {
	page := wid("lg", "layoutGrid1", "Forms$LayoutGrid", map[string]any{
		"Rows": bsonList(map[string]any{
			"$ID": "row", "$Type": "Forms$LayoutGridRow",
			"Columns": bsonList(map[string]any{
				"$ID": "col", "$Type": "Forms$LayoutGridColumn",
				"Widgets": bsonList(
					wid("tc", "tabContainer1", "Forms$TabContainer", map[string]any{
						"TabPages": bsonList(map[string]any{
							"$ID": "tp", "$Type": "Forms$TabPage",
							"Widgets": bsonList(wid("lbl", "label1", "Forms$Label", nil)),
						}),
					}),
				),
			}),
		}),
	})
	assertTree(t, extractWidgetsRecursive(page), map[string]tp{
		"layoutGrid1":   {"", 0},
		"tabContainer1": {"layoutGrid1", 1},
		"label1":        {"tabContainer1", 2},
	})
}

// A list view template IS indexed (#940), so it is a level of its own.
func TestWidgetTree_ListViewTemplate(t *testing.T) {
	lv := wid("lv", "listView1", "Forms$ListView", map[string]any{
		"Widgets": bsonList(wid("t1", "text1", "Forms$DynamicText", nil)),
		"Templates": bsonList(wid("tpl", "", "Forms$ListViewTemplate", map[string]any{
			"Widgets": bsonList(wid("b1", "btnA", "Forms$ActionButton", nil)),
		})),
	})
	rows := extractWidgetsRecursive(lv)
	assertTree(t, rows, map[string]tp{
		"listView1": {"", 0},
		"text1":     {"listView1", 1},
	})
	// A template has no name, so it and its child are checked by ID.
	for _, r := range rows {
		if r.ID == "tpl" && (r.ParentID != "lv" || r.Depth != 1) {
			t.Errorf("template: parent %q depth %d, want lv 1", r.ParentID, r.Depth)
		}
	}
	if btn := widgetByName(rows, "btnA"); btn == nil || btn.ParentID != "tpl" || btn.Depth != 2 {
		t.Errorf("btnA = %+v, want parent tpl at depth 2", btn)
	}
}

// A widget in a DataGrid2 column (an object-list item) is the grid's child, and
// so is one in a child slot (Value.Widgets).
func TestWidgetTree_PluggablePropertyWidgets(t *testing.T) {
	grid := gridWithWidgetInAColumn()
	obj := grid["Object"].(map[string]any)
	obj["Properties"] = append(obj["Properties"].([]any), map[string]any{
		"TypePointer": "t-empty",
		"Value": map[string]any{
			"Widgets": bsonList(wid("slot", "emptyText1", "Forms$Text", nil)),
		},
	})
	page := wid("c", "container1", "Forms$DivContainer", map[string]any{"Widgets": bsonList(grid)})
	assertTree(t, extractWidgetsRecursive(page), map[string]tp{
		"container1": {"", 0},
		"grid1":      {"container1", 1},
		"pb1":        {"grid1", 2},
		"emptyText1": {"grid1", 2},
	})
}

// Page and snippet roots are depth 0 with no parent, including the second and
// later root widgets.
func TestWidgetTree_SnippetRoots(t *testing.T) {
	rows := extractSnippetWidgets(map[string]any{
		"Widgets": bsonList(
			wid("a", "a1", "Forms$Label", nil),
			wid("b", "b1", "Forms$DivContainer", map[string]any{
				"Widgets": bsonList(wid("c", "c1", "Forms$Label", nil)),
			}),
		),
	})
	assertTree(t, rows, map[string]tp{
		"a1": {"", 0},
		"b1": {"", 0},
		"c1": {"b1", 1},
	})
}

func TestWidgetAppearance(t *testing.T) {
	rows := extractWidgetsRecursive(wid("c", "container1", "Forms$DivContainer", map[string]any{
		"Appearance": map[string]any{
			"$Type":          "Forms$Appearance",
			"Class":          "card mx-2",
			"Style":          "color: red;",
			"DynamicClasses": "if $currentObject/Done then 'done' else ''",
		},
	}))
	got := rows[0]
	if got.Class != "card mx-2" || got.Style != "color: red;" ||
		got.DynamicClasses != "if $currentObject/Done then 'done' else ''" {
		t.Errorf("appearance = %q / %q / %q", got.Class, got.Style, got.DynamicClasses)
	}
}

// The primary action is Action, else OnClickAction, else ClickAction; a
// confirmation lives in MicroflowSettings for a microflow call and directly on
// a nanoflow or workflow call. A delete action has no confirmation property.
func TestWidgetPrimaryAction(t *testing.T) {
	confirm := map[string]any{"$Type": "Forms$ConfirmationInfo"}
	cases := []struct {
		name        string
		widget      map[string]any
		wantType    string
		wantConfirm bool
	}{
		{"delete button", map[string]any{"Action": map[string]any{"$Type": "Forms$DeleteClientAction"}},
			"Forms$DeleteClientAction", false},
		{"microflow with confirmation", map[string]any{"Action": map[string]any{
			"$Type":             "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{"$Type": "Forms$MicroflowSettings", "ConfirmationInfo": confirm},
		}}, "Forms$MicroflowAction", true},
		{"microflow without confirmation", map[string]any{"Action": map[string]any{
			"$Type":             "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{"$Type": "Forms$MicroflowSettings", "ConfirmationInfo": nil},
		}}, "Forms$MicroflowAction", false},
		{"nanoflow with confirmation", map[string]any{"Action": map[string]any{
			"$Type": "Forms$CallNanoflowClientAction", "ConfirmationInfo": confirm,
		}}, "Forms$CallNanoflowClientAction", true},
		{"container on-click", map[string]any{"OnClickAction": map[string]any{"$Type": "Forms$FormAction"}},
			"Forms$FormAction", false},
		{"list view click", map[string]any{"ClickAction": map[string]any{"$Type": "Forms$NoAction"}},
			"Forms$NoAction", false},
		{"no action", map[string]any{}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := extractWidgetsRecursive(wid("w", "w1", "Forms$ActionButton", c.widget))
			if rows[0].ActionType != c.wantType || rows[0].HasConfirmation != c.wantConfirm {
				t.Errorf("got (%q, %v), want (%q, %v)", rows[0].ActionType, rows[0].HasConfirmation,
					c.wantType, c.wantConfirm)
			}
		})
	}
}
