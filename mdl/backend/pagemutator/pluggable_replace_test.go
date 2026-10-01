// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1247: REPLACE of a pluggable widget by one of the same
// package keeps every stored property the statement does not change.

func bid(s string) primitive.Binary { return primitive.Binary{Data: []byte(s)} }

// comboDoc builds a pluggable widget the way the three sources differ: each has
// its own element IDs (prefix), and the given property values.
func comboDoc(prefix, editable string, props map[string]string) bson.D {
	keys := []string{"optionsSourceAssociationDataSource", "emptyOptionText", "readOnlyStyle"}
	var pts, ps bson.A
	pts = append(pts, int32(2))
	ps = append(ps, int32(2))
	for _, k := range keys {
		pts = append(pts, bson.D{
			{Key: "$ID", Value: bid(prefix + "pt-" + k)},
			{Key: "PropertyKey", Value: k},
			{Key: "ValueType", Value: bson.D{{Key: "$ID", Value: bid(prefix + "vt-" + k)}}},
		})
		ps = append(ps, bson.D{
			{Key: "$ID", Value: bid(prefix + "p-" + k)},
			{Key: "TypePointer", Value: bid(prefix + "pt-" + k)},
			{Key: "Value", Value: bson.D{
				{Key: "$ID", Value: bid(prefix + "v-" + k)},
				{Key: "PrimitiveValue", Value: props[k]},
				{Key: "TypePointer", Value: bid(prefix + "vt-" + k)},
			}},
		})
	}
	return bson.D{
		{Key: "$ID", Value: bid(prefix + "w")},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Editable", Value: editable},
		{Key: "Name", Value: "comboBox1"},
		{Key: "Object", Value: bson.D{{Key: "$ID", Value: bid(prefix + "o")}, {Key: "Properties", Value: ps}}},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: bid(prefix + "t")},
			{Key: "ObjectType", Value: bson.D{{Key: "$ID", Value: bid(prefix + "ot")}, {Key: "PropertyTypes", Value: pts}}},
			{Key: "WidgetId", Value: "com.mendix.widget.web.combobox.Combobox"},
		}},
	}
}

func propValue(t *testing.T, w bson.D, key string) (string, bson.D) {
	t.Helper()
	p := propertiesByKey(w)[key]
	if p == nil {
		t.Fatalf("no property %s", key)
	}
	v := bsonnav.DGetDoc(p, "Value")
	return bsonnav.DGetString(v, "PrimitiveValue"), v
}

func TestMergeUnstatedPluggable(t *testing.T) {
	stored := comboDoc("S", "Never", map[string]string{
		"optionsSourceAssociationDataSource": "unsorted", "emptyOptionText": "Country", "readOnlyStyle": "text"})
	// Both builds come from the template, so MDL-unmapped properties carry the
	// template's values; only the data source differs — the statement's change.
	baseline := comboDoc("B", "Always", map[string]string{
		"optionsSourceAssociationDataSource": "unsorted", "emptyOptionText": "", "readOnlyStyle": "bordered"})
	replacement := comboDoc("N", "Always", map[string]string{
		"optionsSourceAssociationDataSource": "sorted", "emptyOptionText": "", "readOnlyStyle": "bordered"})

	got, ok := mergeUnstatedPluggable(stored, replacement, baseline)
	if !ok {
		t.Fatal("merge did not apply")
	}
	if v, _ := propValue(t, got, "emptyOptionText"); v != "Country" {
		t.Errorf("emptyOptionText = %q, want the stored \"Country\"", v)
	}
	if v, _ := propValue(t, got, "readOnlyStyle"); v != "text" {
		t.Errorf("readOnlyStyle = %q, want the stored \"text\"", v)
	}
	if e := bsonnav.DGetString(got, "Editable"); e != "Never" {
		t.Errorf("Editable = %q, want the stored \"Never\"", e)
	}
	v, doc := propValue(t, got, "optionsSourceAssociationDataSource")
	if v != "sorted" {
		t.Errorf("data source = %q, want the statement's \"sorted\"", v)
	}
	// The grafted value points into the STORED Type, which is the one kept.
	if tp := idKey(bsonnav.DGet(doc, "TypePointer")); tp != idKey(bid("Svt-optionsSourceAssociationDataSource")) {
		t.Errorf("grafted value's TypePointer = %q, want the stored value type", tp)
	}
	if pluggableWidgetID(got) == "" || idKey(bsonnav.DGet(bsonnav.DGetDoc(got, "Type"), "$ID")) != idKey(bid("St")) {
		t.Error("the stored Type was not kept")
	}
	// The stored document itself is not modified in place.
	if v, _ := propValue(t, stored, "optionsSourceAssociationDataSource"); v != "unsorted" {
		t.Error("merge modified the stored document it was given")
	}

	// A stated change of a widget-level field is applied.
	renamed := comboDoc("N", "Always", map[string]string{
		"optionsSourceAssociationDataSource": "unsorted", "emptyOptionText": "", "readOnlyStyle": "bordered"})
	bsonnav.DSet(renamed, "Name", "cbCountry")
	got, ok = mergeUnstatedPluggable(stored, renamed, baseline)
	if !ok || bsonnav.DGetString(got, "Name") != "cbCountry" {
		t.Errorf("a renaming replace was not applied: ok=%v name=%q", ok, bsonnav.DGetString(got, "Name"))
	}

	// Control: a different widget package is not merged.
	other := comboDoc("N", "Always", nil)
	bsonnav.DSet(bsonnav.DGetDoc(other, "Type"), "WidgetId", "com.mendix.widget.web.datagrid.Datagrid")
	if _, ok := mergeUnstatedPluggable(stored, other, baseline); ok {
		t.Error("merged a widget of another package")
	}
}

// A value that carries a pluggable widget of its own — a DataGrid 2 column's
// filter — is grafted with that widget's pointers left as built: they aim into
// the nested widget's own Type, not the outer one. Re-aiming them failed, and
// the whole REPLACE fell back to the template rebuild.
func TestRemapTypePointersLeavesANestedPluggableWidget(t *testing.T) {
	nested := bson.D{
		{Key: "$ID", Value: bid("f")},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Object", Value: bson.D{{Key: "Properties", Value: bson.A{int32(2),
			bson.D{{Key: "TypePointer", Value: bid("f-pt")}}}}}},
	}
	value := bson.D{
		{Key: "TypePointer", Value: bid("Nvt")},
		{Key: "Widgets", Value: bson.A{int32(2), nested}},
	}
	newPaths := map[string]string{idKey(bid("Nvt")): "/vt"}
	storedIDs := map[string]string{"/vt": idKey(bid("Svt"))}
	got, ok := remapTypePointers(value, newPaths, storedIDs)
	if !ok {
		t.Fatal("a nested pluggable widget's own pointers made the graft fail")
	}
	d := got.(bson.D)
	if idKey(bsonnav.DGet(d, "TypePointer")) != idKey(bid("Svt")) {
		t.Error("the value's own pointer was not re-aimed at the stored Type")
	}
	w := bsonnav.DGetArrayElements(bsonnav.DGet(d, "Widgets"))[0].(bson.D)
	p := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(w, "Object"), "Properties"))[0].(bson.D)
	if idKey(bsonnav.DGet(p, "TypePointer")) != idKey(bid("f-pt")) {
		t.Error("the nested widget's pointer into its own Type was changed")
	}
}

// gridDoc is a pluggable widget with one object-list property, "columns",
// whose objects have "header" and "hidden" properties.
func gridDoc(prefix string, headers, hidden []string) bson.D {
	colType := bson.D{{Key: "$ID", Value: bid(prefix + "cot")}, {Key: "PropertyTypes", Value: bson.A{int32(2),
		bson.D{{Key: "$ID", Value: bid(prefix + "pt-header")}, {Key: "PropertyKey", Value: "header"}},
		bson.D{{Key: "$ID", Value: bid(prefix + "pt-hidden")}, {Key: "PropertyKey", Value: "hidden"}},
	}}}
	objs := bson.A{int32(2)}
	for i := range headers {
		objs = append(objs, bson.D{
			{Key: "$ID", Value: bid(prefix + "col" + headers[i])},
			{Key: "Properties", Value: bson.A{int32(2),
				bson.D{{Key: "TypePointer", Value: bid(prefix + "pt-header")},
					{Key: "Value", Value: bson.D{{Key: "PrimitiveValue", Value: headers[i]}}}},
				bson.D{{Key: "TypePointer", Value: bid(prefix + "pt-hidden")},
					{Key: "Value", Value: bson.D{{Key: "PrimitiveValue", Value: hidden[i]}}}},
			}},
		})
	}
	return bson.D{
		{Key: "$ID", Value: bid(prefix + "w")},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Object", Value: bson.D{{Key: "Properties", Value: bson.A{int32(2),
			bson.D{{Key: "TypePointer", Value: bid(prefix + "pt-columns")},
				{Key: "Value", Value: bson.D{{Key: "Objects", Value: objs}, {Key: "PrimitiveValue", Value: ""}}}},
		}}}},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2),
				bson.D{{Key: "$ID", Value: bid(prefix + "pt-columns")}, {Key: "PropertyKey", Value: "columns"},
					{Key: "ValueType", Value: bson.D{{Key: "ObjectType", Value: colType}}}},
			}}}},
			{Key: "WidgetId", Value: "com.mendix.widget.web.datagrid.Datagrid"},
		}},
	}
}

// Changing one column's header keeps every column's unstated properties: the
// object list is merged object by object, not grafted whole.
func TestMergeUnstatedPluggableMergesAnObjectList(t *testing.T) {
	stored := gridDoc("S", []string{"Name", "Age"}, []string{"yes", "yes"})
	baseline := gridDoc("B", []string{"Name", "Age"}, []string{"no", "no"})
	replacement := gridDoc("N", []string{"Rule name", "Age"}, []string{"no", "no"})
	got, ok := mergeUnstatedPluggable(stored, replacement, baseline)
	if !ok {
		t.Fatal("merge did not apply")
	}
	col := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(bsonnav.DGetArrayElements(
		bsonnav.DGet(bsonnav.DGetDoc(got, "Object"), "Properties"))[0].(bson.D), "Value"), "Objects"))
	if len(col) != 2 {
		t.Fatalf("%d columns, want 2", len(col))
	}
	for i, want := range []string{"Rule name", "Age"} {
		props := bsonnav.DGetArrayElements(bsonnav.DGet(col[i].(bson.D), "Properties"))
		h := bsonnav.DGetString(bsonnav.DGetDoc(props[0].(bson.D), "Value"), "PrimitiveValue")
		hid := bsonnav.DGetString(bsonnav.DGetDoc(props[1].(bson.D), "Value"), "PrimitiveValue")
		if h != want {
			t.Errorf("column %d header = %q, want %q", i, h, want)
		}
		if hid != "yes" {
			t.Errorf("column %d hidden = %q, want the stored \"yes\"", i, hid)
		}
		if idKey(bsonnav.DGet(col[i].(bson.D), "$ID")) != idKey(bid("Scol"+[]string{"Name", "Age"}[i])) {
			t.Errorf("column %d is not the stored column", i)
		}
	}
}
