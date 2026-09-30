// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// attrObject is a widget Object whose one property binds attr.
func attrObject(attr string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "AttributeRef", Value: bson.D{
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: attr},
				}},
			}},
		}}},
	}
}

func comboAST(attr string) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "combobox", Name: "cb1", Properties: map[string]any{"Attribute": attr}}
}

// A widget is kept only when the statement says what describe says about the
// stored one AND the rebuild binds nothing the stored widget does not. Each
// case changes one of those; the first is the control that keeps.
func TestPassStoredThrough(t *testing.T) {
	const widgetID = "com.mendix.widget.web.combobox.Combobox"
	storedType := bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"}, {Key: "WidgetId", Value: widgetID}}
	storedObj := attrObject("M.Order.Status")
	pt := &pluggablePassthrough{byName: map[string]*storedPluggable{
		"cb1": {widgetID: widgetID, typ: storedType, obj: storedObj, declared: comboAST("Status"), refs: modelRefs(storedObj)},
	}}
	def := &WidgetDefinition{WidgetID: widgetID}

	cases := []struct {
		name    string
		def     *WidgetDefinition
		w       *ast.WidgetV3
		rebuilt string // the attribute the rebuild bound
		keep    bool
	}{
		{"unchanged", def, comboAST("Status"), "M.Order.Status", true},
		{"statement changed", def, comboAST("Name"), "M.Order.Name", false},
		// Same text, another context: a data view switched to another entity
		// binds the same bare name to another attribute.
		{"context changed", def, comboAST("Status"), "M.Invoice.Status", false},
		{"another widget package", &WidgetDefinition{WidgetID: "other.Widget"}, comboAST("Status"), "M.Order.Status", false},
		{"no stored widget of that name", def, &ast.WidgetV3{Type: "combobox", Name: "cb2", Properties: map[string]any{"Attribute": "Status"}}, "M.Order.Status", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rebuiltObj := attrObject(c.rebuilt)
			cw := &pages.CustomWidget{RawType: bson.D{{Key: "WidgetId", Value: "template"}}, RawObject: rebuiltObj}
			got := pt.passStoredThrough(c.def, c.w, cw)
			if got != c.keep {
				t.Fatalf("passStoredThrough = %v, want %v", got, c.keep)
			}
			if c.keep {
				if bsonString(cw.RawType, "WidgetId") != widgetID || modelRefs(cw.RawObject)["Attribute=M.Order.Status"] != true {
					t.Errorf("the stored Type and Object were not written: %v %v", cw.RawType, cw.RawObject)
				}
			} else if bsonString(cw.RawType, "WidgetId") != "template" {
				t.Errorf("a rebuilt widget took the stored Type")
			}
		})
	}
}
