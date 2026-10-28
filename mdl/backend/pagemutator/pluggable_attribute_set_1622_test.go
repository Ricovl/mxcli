// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1622: `alter page … set (eventDataAttribute: …) on chart1` was
// refused for every spelling — "pluggable property "eventDataAttribute" holds a
// value of kind Attribute, which ALTER cannot set from a plain value". The
// mutator holds only BSON, so it cannot tell which entity a bare name belongs
// to; the executor resolves the binding and hands it an AttributeBinding, and
// the mutator reports what it needs for that: the property's kind, its
// datasource link, and the scope of the linked and the enclosing sources.

// chartInDataView is a data view over M.Entry holding a Custom chart whose
// dataAttribute is linked to the chart's own datasource (over M.Point) and
// whose eventDataAttribute is linked to none, so it binds to the data view.
func chartInDataView() bson.D {
	dsID, dataID, eventID := idBin(0x40), idBin(0x41), idBin(0x42)
	propType := func(id bson.Binary, key, kind, link string) bson.D {
		vt := bson.D{{Key: "Type", Value: kind}}
		if link != "" {
			vt = append(vt, bson.E{Key: "DataSourceProperty", Value: link})
		}
		return bson.D{{Key: "$ID", Value: id}, {Key: "PropertyKey", Value: key}, {Key: "ValueType", Value: vt}}
	}
	attrValue := func(attr string) bson.D {
		var ref any
		if attr != "" {
			ref = bson.D{
				{Key: "$ID", Value: idBin(0x50)},
				{Key: "$Type", Value: "DomainModels$AttributeRef"},
				{Key: "Attribute", Value: attr},
				{Key: "EntityRef", Value: nil},
			}
		}
		return bson.D{
			{Key: "AttributeRef", Value: ref},
			{Key: "Expression", Value: ""},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "SourceVariable", Value: nil},
		}
	}
	chart := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "chart1"},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{int32(2),
					propType(dsID, "datasource", "DataSource", ""),
					propType(dataID, "dataAttribute", "Attribute", "datasource"),
					propType(eventID, "eventDataAttribute", "Attribute", ""),
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{int32(2),
				bson.D{{Key: "TypePointer", Value: dsID}, {Key: "Value", Value: bson.D{
					{Key: "DataSource", Value: bson.D{{Key: "EntityRef", Value: directEntityRef("M.Point")}}},
				}}},
				bson.D{{Key: "TypePointer", Value: dataID}, {Key: "Value", Value: attrValue("")}},
				bson.D{{Key: "TypePointer", Value: eventID}, {Key: "Value", Value: attrValue("M.Entry.Old")}},
			}},
		}},
	}
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvEntry"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: directEntityRef("M.Entry")},
		}},
		{Key: "Widgets", Value: bson.A{int32(2), chart}},
	}
	return pageWith(dv)
}

func chartValue(t *testing.T, m *Mutator, key string) bson.D {
	t.Helper()
	result := m.widgetFinder(m.rawData, "chart1")
	if result == nil {
		t.Fatal("chart1 not found")
	}
	keys := buildPropKeyMap(result.widget)
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(result.widget, "Object"), "Properties")) {
		pd := p.(bson.D)
		if keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))] == key {
			return bsonnav.DGetDoc(pd, "Value")
		}
	}
	t.Fatalf("no property %s", key)
	return nil
}

func TestPluggablePropertyScope(t *testing.T) {
	m := &Mutator{rawData: chartInDataView(), widgetFinder: findBsonWidget}

	got, ok := m.PluggablePropertyScope("chart1", "eventDataAttribute")
	want := backend.PluggablePropertyScope{Key: "eventDataAttribute", Kind: "Attribute",
		Enclosing: backend.SourceScope{Entity: "M.Entry"}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("eventDataAttribute: got %+v, %v; want %+v", got, ok, want)
	}

	// Linked to the chart's own datasource; the key matched case-insensitively,
	// as the setter matches it (#1069).
	got, ok = m.PluggablePropertyScope("chart1", "DataAttribute")
	want = backend.PluggablePropertyScope{Key: "dataAttribute", Kind: "Attribute", DataSource: "datasource",
		Linked: backend.SourceScope{Entity: "M.Point"}, Enclosing: backend.SourceScope{Entity: "M.Entry"}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("dataAttribute: got %+v, %v; want %+v", got, ok, want)
	}

	for _, c := range []struct{ widget, key string }{{"chart1", "noSuchKey"}, {"dvEntry", "Name"}, {"nope", "x"}} {
		if _, ok := m.PluggablePropertyScope(c.widget, c.key); ok {
			t.Errorf("%s.%s: ok, want false", c.widget, c.key)
		}
	}
}

func TestSetPluggableAttributeBinding(t *testing.T) {
	m := &Mutator{rawData: chartInDataView(), widgetFinder: findBsonWidget}

	// `$Ctx.EventData`: the AttributeRef plus the parameter it is read from.
	err := m.SetWidgetProperty("chart1", "eventDataAttribute", backend.AttributeBinding{
		Path: "M.Context.EventData", SourceVariable: &pages.WidgetVariable{Variable: "Ctx"}})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	val := chartValue(t, m, "eventDataAttribute")
	ref := bsonnav.DGetDoc(val, "AttributeRef")
	if bsonnav.DGetString(ref, "$Type") != "DomainModels$AttributeRef" ||
		bsonnav.DGetString(ref, "Attribute") != "M.Context.EventData" || bsonnav.DGet(ref, "EntityRef") != nil {
		t.Errorf("AttributeRef = %v, want DomainModels$AttributeRef M.Context.EventData with a null EntityRef", ref)
	}
	sv := bsonnav.DGetDoc(val, "SourceVariable")
	if bsonnav.DGetString(sv, "$Type") != "Forms$PageVariable" || bsonnav.DGetString(sv, "PageParameter") != "Ctx" {
		t.Errorf("SourceVariable = %v, want a Forms$PageVariable naming page parameter Ctx", sv)
	}

	// Bound back to the context object: the SourceVariable goes, or the value
	// would still be read from $Ctx.
	if err := m.SetWidgetProperty("chart1", "eventDataAttribute", backend.AttributeBinding{Path: "M.Entry.JsonData"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	val = chartValue(t, m, "eventDataAttribute")
	if got := bsonnav.DGetString(bsonnav.DGetDoc(val, "AttributeRef"), "Attribute"); got != "M.Entry.JsonData" {
		t.Errorf("Attribute = %q, want M.Entry.JsonData", got)
	}
	if sv := bsonnav.DGet(val, "SourceVariable"); sv != nil {
		t.Errorf("SourceVariable = %v, want null", sv)
	}

	// An empty AttributeRef slot is filled, not skipped.
	if err := m.SetWidgetProperty("chart1", "dataAttribute", backend.AttributeBinding{Path: "M.Point.X"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := bsonnav.DGetString(bsonnav.DGetDoc(chartValue(t, m, "dataAttribute"), "AttributeRef"), "Attribute"); got != "M.Point.X" {
		t.Errorf("dataAttribute = %q, want M.Point.X", got)
	}
}

// Control: an unresolved value on an Attribute property is still refused — the
// mutator cannot tell which entity a bare name belongs to.
func TestSetPluggableAttributePlainValueStillRefused(t *testing.T) {
	m := &Mutator{rawData: chartInDataView(), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("chart1", "eventDataAttribute", "JsonData")
	if err == nil || !strings.Contains(err.Error(), "kind Attribute") {
		t.Fatalf("want the kind-Attribute refusal, got %v", err)
	}
	if got := bsonnav.DGetString(bsonnav.DGetDoc(chartValue(t, m, "eventDataAttribute"), "AttributeRef"), "Attribute"); got != "M.Entry.Old" {
		t.Errorf("Attribute = %q after a refusal, want the stored M.Entry.Old", got)
	}
}

// An AttributeBinding on a property of another kind is a caller bug, refused
// rather than stringified into a PrimitiveValue.
func TestSetPluggableAttributeBindingOnOtherKindRefused(t *testing.T) {
	m := &Mutator{rawData: chartInDataView(), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("chart1", "datasource", backend.AttributeBinding{Path: "M.Point.X"}); err == nil {
		t.Fatal("an attribute binding on a DataSource property was accepted")
	}
}
