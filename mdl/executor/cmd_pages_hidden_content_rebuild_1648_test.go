// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1648: `create or modify` of a page whose combo box the statement
// CHANGES rebuilt the combo box from the template and deleted its custom
// content — txtOptionName, which MDL cannot author — while the content type
// stayed 'yes': "Replaced page", then mx check CE0642 "Property 'Custom
// content' is required." An unchanged combo box survived through the pluggable
// passthrough, and `alter page … set` keeps it, so the refusal is for the
// rebuild alone (ADR-0012: carry or refuse, never drop).

func hcBin(s string) bson.Binary { return bson.Binary{Data: []byte(s)} }

// storedComboWithContent is a stored combo box Type and Object whose custom
// content holds the named widgets.
func storedComboWithContent(content ...string) (typ, obj bson.D) {
	widgets := bson.A{int32(2)}
	for _, n := range content {
		widgets = append(widgets, bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: n}})
	}
	typ = bson.D{{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2),
		bson.D{{Key: "$ID", Value: hcBin("pt-label")}, {Key: "PropertyKey", Value: "emptyOptionText"},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "TextTemplate"}}}},
		bson.D{{Key: "$ID", Value: hcBin("pt-content")}, {Key: "PropertyKey", Value: "optionsSourceAssociationCustomContent"},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "Widgets"}, {Key: "DataSourceProperty", Value: "optionsSourceAssociationDataSource"}}}},
	}}}}}
	obj = bson.D{{Key: "Properties", Value: bson.A{int32(2),
		bson.D{{Key: "TypePointer", Value: hcBin("pt-label")}, {Key: "Value", Value: bson.D{{Key: "Widgets", Value: bson.A{int32(2)}}}}},
		bson.D{{Key: "TypePointer", Value: hcBin("pt-content")}, {Key: "Value", Value: bson.D{{Key: "Widgets", Value: widgets}}}},
	}}}
	return typ, obj
}

const describedCombo = `combobox cbBooking (
  Label: 'Booking',
  Attribute: Holder_Booking,
  DataSource: database from MyFirstModule.Booking,
  CaptionAttribute: Name,
  optionsSourceAssociationCustomContentType: 'yes'
)`

func passthroughWith(t *testing.T, content ...string) *pluggablePassthrough {
	t.Helper()
	typ, obj := storedComboWithContent(content...)
	return &pluggablePassthrough{byName: map[string]*storedPluggable{
		"cbBooking": {
			widgetID: comboBoxPackageWidgetID,
			typ:      typ, obj: obj,
			declared: parseSnippetWidget(t, describedCombo),
			hidden:   storedHiddenContent(comboBoxPackageWidgetID, typ, obj),
		},
	}}
}

func TestStoredHiddenContentReadsTheComboBoxContent(t *testing.T) {
	typ, obj := storedComboWithContent("txtOptionName")
	got := storedHiddenContent(comboBoxPackageWidgetID, typ, obj)
	if len(got) != 1 || got[0].Property != "optionsSourceAssociationCustomContent" ||
		strings.Join(got[0].Widgets, ",") != "txtOptionName" {
		t.Errorf("storedHiddenContent = %+v", got)
	}
	// Another package's widgets property is not this rule's: its slots are
	// described, and a widget left out of the statement is a deletion.
	if got := storedHiddenContent("com.mendix.widget.web.gallery.Gallery", typ, obj); len(got) != 0 {
		t.Errorf("gallery reported hidden content %+v", got)
	}
}

func TestChangedComboBoxWithHiddenContentIsRefused(t *testing.T) {
	pt := passthroughWith(t, "txtOptionName")
	changed := parseSnippetWidget(t, strings.Replace(describedCombo, "Label: 'Booking'", "Label: 'Booking (changed)'", 1))
	err := pt.refuseChangedWithHiddenContent([]*ast.WidgetV3{changed})
	if err == nil {
		t.Fatal("a rebuild deleting custom content was not refused")
	}
	for _, want := range []string{"cbBooking", "txtOptionName", "optionsSourceAssociationCustomContent", "alter page", "drop txtOptionName"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal lacks %q: %v", want, err)
		}
	}
}

func TestComboBoxWithHiddenContentControls(t *testing.T) {
	// Unchanged: passed through, content kept.
	if err := passthroughWith(t, "txtOptionName").refuseChangedWithHiddenContent(
		[]*ast.WidgetV3{parseSnippetWidget(t, describedCombo)}); err != nil {
		t.Errorf("unchanged combo box refused: %v", err)
	}
	// No content to lose.
	changed := parseSnippetWidget(t, strings.Replace(describedCombo, "Label: 'Booking'", "Label: 'Other'", 1))
	if err := passthroughWith(t).refuseChangedWithHiddenContent([]*ast.WidgetV3{changed}); err != nil {
		t.Errorf("combo box without content refused: %v", err)
	}
	// Another widget kind under the name: the author replaced the combo box.
	other := parseSnippetWidget(t, `textbox cbBooking (Label: 'Booking', Attribute: Note)`)
	if err := passthroughWith(t, "txtOptionName").refuseChangedWithHiddenContent([]*ast.WidgetV3{other}); err != nil {
		t.Errorf("a different widget under the name refused: %v", err)
	}
	// Nested: the changed combo box inside a data view is found.
	nested := parseSnippetWidget(t, "dataview dv (DataSource: $Holder) {\n"+
		strings.Replace(describedCombo, "Label: 'Booking'", "Label: 'X'", 1)+"\n}")
	if err := passthroughWith(t, "txtOptionName").refuseChangedWithHiddenContent([]*ast.WidgetV3{nested}); err == nil {
		t.Error("a changed combo box nested in a data view was not refused")
	}
}
