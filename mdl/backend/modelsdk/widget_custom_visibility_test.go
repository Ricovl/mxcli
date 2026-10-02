// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"reflect"
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A pluggable widget's visibility and editability are stored on the
// CustomWidgets$CustomWidget, in the same Forms$Conditional*Settings elements a
// text box carries (Studio Pro-authored combo boxes, 11.14). So the shape must
// be the text box's, key for key — a combo box written any other way is a
// document Studio Pro did not write.
func TestCustomWidgetVisibilityEditability_SameShapeAsTextBox(t *testing.T) {
	const expr = "$currentObject/Name != empty"
	cv := func() *pages.ConditionalVisibilitySettings {
		return &pages.ConditionalVisibilitySettings{
			BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"}, Expression: expr}
	}
	ce := func() *pages.ConditionalEditabilitySettings {
		return &pages.ConditionalEditabilitySettings{
			BaseElement: model.BaseElement{TypeName: "Forms$ConditionalEditabilitySettings"}, Expression: expr}
	}

	tb := &pages.TextBox{}
	tb.Name = "tb"
	tb.ConditionalVisibility = cv()
	tb.ConditionalEditability = ce()

	cw := &pages.CustomWidget{Editable: "Conditional"}
	cw.Name = "cmb"
	cw.ConditionalVisibility = cv()
	cw.ConditionalEditability = ce()

	tbDoc, cwDoc := encodeWidget(t, tb), encodeWidget(t, cw)
	if got := docGet(cwDoc, "Editable"); got != "Conditional" {
		t.Errorf("CustomWidget Editable = %v, want Conditional", got)
	}
	for _, key := range []string{"ConditionalVisibilitySettings", "ConditionalEditabilitySettings"} {
		want, ok := docGet(tbDoc, key).(bsonv1.D)
		if !ok {
			t.Fatalf("text box %s not serialized: %T", key, docGet(tbDoc, key))
		}
		got, ok := docGet(cwDoc, key).(bsonv1.D)
		if !ok {
			t.Fatalf("custom widget %s not serialized: %T", key, docGet(cwDoc, key))
		}
		if !reflect.DeepEqual(shape(got), shape(want)) {
			t.Errorf("%s shape differs from the text box's:\n got  %v\n want %v", key, shape(got), shape(want))
		}
		if docGet(got, "Expression") != expr {
			t.Errorf("%s Expression = %v, want %q", key, docGet(got, "Expression"), expr)
		}
	}
}

// shape is a document's keys with their values, minus $ID.
func shape(d bsonv1.D) []string {
	var out []string
	for _, e := range d {
		if e.Key == "$ID" {
			continue
		}
		out = append(out, fmt.Sprintf("%s=%T:%v", e.Key, e.Value, e.Value))
	}
	return out
}
