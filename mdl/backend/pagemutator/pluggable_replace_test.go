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
