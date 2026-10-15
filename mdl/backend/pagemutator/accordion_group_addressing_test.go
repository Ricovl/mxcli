// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ---------------------------------------------------------------------------
// #891 / ako/mxcli#1274: the ALTER PAGE resolver did not descend into a
// pluggable widget's object-list items (Accordion groups, Pop-up menu items),
// so a DataGrid 2 inside an Accordion group could not be addressed — neither the
// grid by its name nor its columns by their derived names — while the not-found
// message, built from a generic walk that does descend, listed those very
// columns as available:
//
//	set (Caption: 'Quantity') on Qty
//	  -> widget "Qty" not found ... available columns: Name, Qty
//	set (Caption: 'Quantity') on g1.Qty
//	  -> widget "g1" not found
// ---------------------------------------------------------------------------

// accordionWithGroupContent builds the minimal Accordion BSON the resolver has
// to traverse: a `groups` object list whose single group keeps its widgets at
// Objects[].Properties[content].Value.Widgets — the same depth a DataGrid 2
// column's customContent sits at, but under a property that is not `columns`.
func accordionWithGroupContent(groupWidgets ...bson.D) bson.D {
	groupsID := idBin(0x30)
	contentID := idBin(0x31)

	widgets := bson.A{int32(2)}
	for _, w := range groupWidgets {
		widgets = append(widgets, w)
	}
	group := bson.D{
		{Key: "$Type", Value: objectListItemType},
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "TypePointer", Value: contentID},
				{Key: "Value", Value: bson.D{{Key: "Widgets", Value: widgets}}},
			},
		}},
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "accordion1"},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: groupsID},
						{Key: "PropertyKey", Value: "groups"},
						{Key: "ValueType", Value: bson.D{
							{Key: "ObjectType", Value: bson.D{
								{Key: "PropertyTypes", Value: bson.A{
									int32(2),
									bson.D{{Key: "$ID", Value: contentID}, {Key: "PropertyKey", Value: "content"}},
								}},
							}},
						}},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: groupsID},
					{Key: "Value", Value: bson.D{{Key: "Objects", Value: bson.A{int32(2), group}}}},
				},
			}},
		}},
	}
}

// gridWithCaptionColumns builds a DataGrid 2 whose columns are addressed by
// caption, with `header` declared as a TextTemplate the way the shipped widget
// declares it — so the caption setter edits the stored translation in place.
func gridWithCaptionColumns(name string, captions ...string) bson.D {
	colsID := idBin(0x50)
	headerID := idBin(0x51)
	cols := bson.A{int32(2)}
	for _, c := range captions {
		cols = append(cols, bson.D{
			{Key: "$Type", Value: objectListItemType},
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: headerID},
					{Key: "Value", Value: bson.D{
						{Key: "TextTemplate", Value: bson.D{
							{Key: "Template", Value: bson.D{
								{Key: "Items", Value: bson.A{int32(3), bson.D{
									{Key: "$Type", Value: "Texts$Translation"},
									{Key: "LanguageCode", Value: model.AuthoringLanguage()},
									{Key: "Text", Value: c},
								}}},
							}},
						}},
					}},
				},
			}},
		})
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: colsID},
						{Key: "PropertyKey", Value: "columns"},
						{Key: "ValueType", Value: bson.D{
							{Key: "ObjectType", Value: bson.D{
								{Key: "PropertyTypes", Value: bson.A{
									int32(2),
									bson.D{
										{Key: "$ID", Value: headerID},
										{Key: "PropertyKey", Value: "header"},
										{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "TextTemplate"}}},
									},
								}},
							}},
						}},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: colsID},
					{Key: "Value", Value: bson.D{{Key: "Objects", Value: cols}}},
				},
			}},
		}},
	}
}

func accordionGridPage() bson.D {
	grid := gridWithCaptionColumns("g1", "Name", "Qty")
	return pageWith(accordionWithGroupContent(grid))
}

// treeHolds reports whether a string value equal to want sits anywhere in the
// tree — enough to show the write landed in the stored page, under the
// Accordion, rather than in a detached copy.
func treeHolds(v any, want string) bool {
	switch x := v.(type) {
	case string:
		return x == want
	case bson.D:
		for _, e := range x {
			if treeHolds(e.Value, want) {
				return true
			}
		}
	case bson.A:
		for _, e := range x {
			if treeHolds(e, want) {
				return true
			}
		}
	}
	return false
}

// The grid itself must be reachable by name through the Accordion group.
func TestFindBsonWidget_GridInsideAccordionGroup(t *testing.T) {
	if res := findBsonWidget(accordionGridPage(), "g1"); res == nil {
		t.Fatal(`widget "g1" inside an Accordion group was not found`)
	}
}

// The qualified form: `set (Caption: 'Quantity') on g1.Qty`.
func TestSetColumnProperty_GridInsideAccordionGroup(t *testing.T) {
	m := &Mutator{rawData: accordionGridPage(), widgetFinder: findBsonWidget}
	if err := m.SetColumnProperty("g1", "Qty", "Caption", "Quantity"); err != nil {
		t.Fatalf("set Caption on g1.Qty: %v", err)
	}
	if !treeHolds(m.rawData, "Quantity") {
		t.Error("the caption write did not land in the stored page")
	}
}

// The bare form: `set (Caption: 'Quantity') on Qty` — the message used to list
// Qty as available and then report it not found.
func TestSetWidgetProperty_ColumnInsideAccordionGroup(t *testing.T) {
	m := &Mutator{rawData: accordionGridPage(), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("Qty", "Caption", "Quantity"); err != nil {
		t.Fatalf("set Caption on Qty: %v", err)
	}
	if !treeHolds(m.rawData, "Quantity") {
		t.Error("the caption write did not land in the stored page")
	}
}

// A plain widget inside a group (not a grid) is the same descent, and the
// widget scope `check` uses must see it too.
func TestAccordionGroupWidgets_InFinderAndScope(t *testing.T) {
	txt := bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "$ID", Value: idBin(0x40)},
		{Key: "Name", Value: "txtInGroup"},
	}
	raw := pageWith(accordionWithGroupContent(txt))
	if res := findBsonWidget(raw, "txtInGroup"); res == nil {
		t.Fatal(`widget "txtInGroup" inside an Accordion group was not found`)
	}
	scope := extractWidgetScopeFromBSON(raw)
	if _, ok := scope["txtInGroup"]; !ok {
		t.Errorf("widget scope misses txtInGroup: %v", scope)
	}
}

// A miss must still report the addressable names — the descent must not turn a
// mistyped column into a silent no-op.
func TestSetWidgetProperty_AccordionGroupMissStillListsColumns(t *testing.T) {
	m := &Mutator{rawData: accordionGridPage(), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("Qtyy", "Caption", "x")
	if err == nil || !strings.Contains(err.Error(), "Qty") {
		t.Fatalf("expected a not-found listing Qty, got %v", err)
	}
}
