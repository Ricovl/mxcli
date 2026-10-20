// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1459: a legacy Forms$DataGrid's columns and search fields
// carry a stored Name, but the widget finder only searched the control bar, so
// `alter page … drop column14` passed check and failed at exec with
// `widget "column14" not found`.

func legacyGridText(s string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: "en_US"},
			{Key: "Text", Value: s},
		}}},
	}
}

func legacyGridAttrRef(qn string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "DomainModels$AttributeRef"},
		{Key: "Attribute", Value: qn},
		{Key: "EntityRef", Value: nil},
	}
}

func legacyGridColumn(name, caption, attr string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$GridColumn"},
		{Key: "Name", Value: name},
		{Key: "Caption", Value: legacyGridText(caption)},
		{Key: "AttributeRef", Value: legacyGridAttrRef(attr)},
		{Key: "Editable", Value: false},
		{Key: "Width", Value: int32(50)},
	}
}

func legacyGridSearchField(typ, name, caption, attr string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
		{Key: "Caption", Value: legacyGridText(caption)},
		{Key: "Type", Value: "Normal"},
		{Key: "AttributeRef", Value: legacyGridAttrRef(attr)},
	}
}

func makeLegacyGridPage() bson.D {
	return makeRawPage(bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$DataGrid"},
		{Key: "Name", Value: "grid1"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$GridXPathSource"},
			{Key: "SearchBar", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$SearchBar"},
				{Key: "Items", Value: bson.A{int32(2),
					legacyGridSearchField("Forms$ComparisonSearchField", "searchField1", "Number", "Shop.Order.Number"),
					legacyGridSearchField("Forms$DropDownSearchField", "searchField2", "Note", "Shop.Order.Note"),
				}},
			}},
		}},
		{Key: "ControlBar", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$GridControlBar"},
			{Key: "Items", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "Forms$GridNewButton"},
				{Key: "Name", Value: "newButton1"},
				{Key: "Caption", Value: legacyGridText("New")},
			}}},
		}},
		{Key: "Columns", Value: bson.A{int32(2),
			legacyGridColumn("column1", "Order number", "Shop.Order.Number"),
			legacyGridColumn("column14", "Internal note", "Shop.Order.Note"),
			legacyGridColumn("column2", "Date", "Shop.Order.Date"),
		}},
	})
}

func legacyGridDoc(t *testing.T, raw bson.D) bson.D {
	t.Helper()
	r := findBsonWidget(raw, "grid1")
	if r == nil {
		t.Fatal("grid1 no longer resolves")
	}
	return r.widget
}

func legacyGridSearchItems(t *testing.T, raw bson.D) bson.A {
	t.Helper()
	ds := bsonnav.DGetDoc(legacyGridDoc(t, raw), "DataSource")
	return bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(ds, "SearchBar"), "Items"))
}

func marshalDoc(t *testing.T, v any) []byte {
	t.Helper()
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// itemNames lists the Name of every document in a stored list, and fails when
// the list lost its leading type marker.
func itemNames(t *testing.T, arr bson.A) []string {
	t.Helper()
	if len(arr) == 0 || !isListMarker(arr[0]) {
		t.Fatalf("list lost its marker: %v", arr)
	}
	var out []string
	for _, e := range arr[1:] {
		out = append(out, bsonnav.DGetString(e.(bson.D), "Name"))
	}
	return out
}

func TestDropLegacyGridColumnRemovesOnlyThatColumn(t *testing.T) {
	raw := makeLegacyGridPage()
	before := bsonnav.ToBsonA(bsonnav.DGet(legacyGridDoc(t, raw), "Columns"))
	col1, col2 := marshalDoc(t, before[1]), marshalDoc(t, before[3])

	m := New(raw, model.ID("page-1"), &stubWidgetDeps{})
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "column14"}}); err != nil {
		t.Fatalf("drop column14: %v", err)
	}
	after := bsonnav.ToBsonA(bsonnav.DGet(legacyGridDoc(t, m.rawData), "Columns"))
	if got := itemNames(t, after); len(got) != 2 || got[0] != "column1" || got[1] != "column2" {
		t.Fatalf("columns after drop = %v, want [column1 column2]", got)
	}
	if !bytes.Equal(marshalDoc(t, after[1]), col1) || !bytes.Equal(marshalDoc(t, after[2]), col2) {
		t.Error("a sibling column changed: it must pass through as stored")
	}
	if got := itemNames(t, legacyGridSearchItems(t, m.rawData)); len(got) != 2 {
		t.Errorf("search fields changed by a column drop: %v", got)
	}
}

func TestDropLegacyGridSearchFieldRemovesOnlyThatField(t *testing.T) {
	raw := makeLegacyGridPage()
	keep := marshalDoc(t, legacyGridSearchItems(t, raw)[1])

	m := New(raw, model.ID("page-1"), &stubWidgetDeps{})
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "searchField2"}}); err != nil {
		t.Fatalf("drop searchField2: %v", err)
	}
	items := legacyGridSearchItems(t, m.rawData)
	if got := itemNames(t, items); len(got) != 1 || got[0] != "searchField1" {
		t.Fatalf("search fields after drop = %v, want [searchField1]", got)
	}
	if !bytes.Equal(marshalDoc(t, items[1]), keep) {
		t.Error("the remaining search field changed")
	}
	if got := itemNames(t, bsonnav.ToBsonA(bsonnav.DGet(legacyGridDoc(t, m.rawData), "Columns"))); len(got) != 3 {
		t.Errorf("columns changed by a search-field drop: %v", got)
	}
}

func TestSetCaptionOnLegacyGridColumnAndSearchField(t *testing.T) {
	m := New(makeLegacyGridPage(), model.ID("page-1"), &stubWidgetDeps{})
	for name, caption := range map[string]string{"column14": "Note", "searchField1": "Order no."} {
		if err := m.SetWidgetProperty(name, "Caption", caption); err != nil {
			t.Fatalf("set Caption on %s: %v", name, err)
		}
		r := findBsonWidget(m.rawData, name)
		if r == nil {
			t.Fatalf("%s no longer resolves", name)
		}
		tr := bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(r.widget, "Caption"), "Items"))
		if got := bsonnav.DGetString(tr[1].(bson.D), "Text"); got != caption {
			t.Errorf("%s caption = %q, want %q", name, got, caption)
		}
	}
	if !m.FindWidget("column2") || m.WidgetScope()["searchField2"] == "" {
		t.Error("legacy grid columns and search fields are not in the widget scope")
	}
}
