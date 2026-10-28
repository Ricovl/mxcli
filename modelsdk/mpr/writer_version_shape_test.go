// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ako/mxcli#1209: mxcli builds three page properties in the shape Mendix 10.11
// stores, and Mendix stored each differently before a version boundary —
// measured on Studio Pro-authored 9.24.30 pages and in mendixmodelsdk:
//
//	Forms$DesignPropertyValue  Value {Toggle|Option…}  → Type/StringValue/BooleanValue below 10.2
//	Forms$DataView             Editability             → Editable (bool) below 10.7
//	TextBox/TextArea/DatePicker PlaceholderTemplate     → Placeholder (Texts$Text) below 10.11
//
// Every unit write is normalized for the project's version, so the shape is
// lowered there, for every writer at once.

func bid() bson.Binary { return bson.Binary{Subtype: 0, Data: make([]byte, 16)} }

func text(s string) bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: s}}}}}
}

func clientTemplate(s string, params ...bson.D) bson.D {
	ps := bson.A{int32(2)}
	for _, p := range params {
		ps = append(ps, p)
	}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Fallback", Value: text("")}, {Key: "Parameters", Value: ps}, {Key: "Template", Value: text(s)}}
}

func designProperty(key string, value bson.D) bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$DesignPropertyValue"},
		{Key: "Key", Value: key}, {Key: "Value", Value: value}}
}

func toggleValue() bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$ToggleDesignPropertyValue"}}
}

func optionValue(o string) bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$OptionDesignPropertyValue"}, {Key: "Option", Value: o}}
}

// shapeSnippet is a snippet as mxcli builds it: a read-only data view holding
// a text box with a placeholder and two design properties.
func shapeSnippet(editability string, dps bson.A, placeholder bson.D) bson.D {
	textBox := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Appearance", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""}, {Key: "DesignProperties", Value: dps}, {Key: "DynamicClasses", Value: ""}, {Key: "Style", Value: ""}}},
		{Key: "Name", Value: "txtEmail"},
		{Key: "PlaceholderTemplate", Value: placeholder},
		{Key: "ReadOnlyStyle", Value: "Inherit"}}
	dataView := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$DataView"},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "Editability", Value: editability},
		{Key: "Name", Value: "dv"},
		{Key: "Widgets", Value: bson.A{int32(2), textBox}}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Snippet"},
		{Key: "Name", Value: "S"}, {Key: "Widgets", Value: bson.A{int32(2), dataView}}}
}

func defaultDesignProperties() bson.A {
	return bson.A{int32(3), designProperty("Hover style", toggleValue()), designProperty("Spacing bottom", optionValue("Outer small"))}
}

func normalizeShape(t *testing.T, v *version.ProjectVersion, doc bson.D) (bson.D, error) {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	out, err := normalizePropertySetsFor(v, "unit", raw)
	if err != nil {
		return nil, err
	}
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	return d, nil
}

// find returns the first document in v whose $Type is typeName.
func find(v any, typeName string) bson.D {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if e.Key == "$Type" && e.Value == typeName {
				return x
			}
		}
		for _, e := range x {
			if d := find(e.Value, typeName); d != nil {
				return d
			}
		}
	case bson.A:
		for _, el := range x {
			if d := find(el, typeName); d != nil {
				return d
			}
		}
	}
	return nil
}

func keysOf(d bson.D) []string {
	var ks []string
	for _, e := range d {
		ks = append(ks, e.Key)
	}
	return ks
}

func get(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

func designPropertyEntries(t *testing.T, d bson.D) []bson.D {
	t.Helper()
	app, _ := get(find(d, "Forms$TextBox"), "Appearance")
	list, _ := get(app.(bson.D), "DesignProperties")
	var out []bson.D
	for _, el := range list.(bson.A)[1:] {
		out = append(out, el.(bson.D))
	}
	return out
}

func TestNormalize_WritesMendix9Shapes(t *testing.T) {
	d, err := normalizeShape(t, pv(9, 24, 30), shapeSnippet("Never", defaultDesignProperties(), clientTemplate("you@example.com")))
	if err != nil {
		t.Fatalf("a snippet with a read-only data view, a placeholder and two design properties is refused on 9.24: %v", err)
	}

	// The design properties, as Studio Pro 9.24.30 stores them (key order too).
	dps := designPropertyEntries(t, d)
	want := [][]string{
		{"$ID", "$Type", "BooleanValue", "Key", "StringValue", "Type"},
		{"$ID", "$Type", "BooleanValue", "Key", "StringValue", "Type"},
	}
	for i, dp := range dps {
		if got := strings.Join(keysOf(dp), ","); got != strings.Join(want[i], ",") {
			t.Errorf("design property %d keys = %s, want %s", i, got, strings.Join(want[i], ","))
		}
	}
	if typ, _ := get(dps[0], "Type"); typ != "Toggle" {
		t.Errorf("toggle stored as Type %v, want Toggle", typ)
	}
	if b, _ := get(dps[0], "BooleanValue"); b != true {
		t.Errorf("toggle stored with BooleanValue %v, want true", b)
	}
	if typ, _ := get(dps[1], "Type"); typ != "DropDown" {
		t.Errorf("option stored as Type %v, want DropDown", typ)
	}
	if s, _ := get(dps[1], "StringValue"); s != "Outer small" {
		t.Errorf("option stored with StringValue %v, want Outer small", s)
	}

	// The data view's editability, as a boolean.
	dv := find(d, "Forms$DataView")
	if _, ok := get(dv, "Editability"); ok {
		t.Errorf("data view keeps Editability, which 9.24 does not declare: %v", keysOf(dv))
	}
	if e, ok := get(dv, "Editable"); !ok || e != false {
		t.Errorf("data view Editable = %v (present %v), want false for Never", e, ok)
	}

	// The text box's placeholder, as a Texts$Text.
	tb := find(d, "Forms$TextBox")
	if _, ok := get(tb, "PlaceholderTemplate"); ok {
		t.Errorf("text box keeps PlaceholderTemplate, which 9.24 does not declare")
	}
	ph, ok := get(tb, "Placeholder")
	if !ok {
		t.Fatalf("text box has no Placeholder: %v", keysOf(tb))
	}
	if typ, _ := get(ph.(bson.D), "$Type"); typ != "Texts$Text" {
		t.Errorf("Placeholder is a %v, want Texts$Text", typ)
	}
	if !strings.Contains(bson.Raw(mustMarshal(t, ph.(bson.D))).String(), "you@example.com") {
		t.Errorf("Placeholder lost its text: %v", ph)
	}
}

// Each shape changes at its own version; on either side of one boundary the
// other two stay as they are.
func TestNormalize_VersionShapeBoundaries(t *testing.T) {
	for _, tt := range []struct {
		v                                      *version.ProjectVersion
		flatDesignProps, editable, placeholder bool
	}{
		{pv(10, 1, 0), true, true, true},
		{pv(10, 2, 0), false, true, true},
		{pv(10, 6, 0), false, true, true},
		{pv(10, 7, 0), false, false, true},
		{pv(10, 10, 0), false, false, true},
		{pv(10, 11, 0), false, false, false},
		{pv(10, 24, 24), false, false, false},
		{pv(11, 13, 0), false, false, false}, // control
	} {
		d, err := normalizeShape(t, tt.v, shapeSnippet("Always", defaultDesignProperties(), clientTemplate("hint")))
		if err != nil {
			t.Errorf("%d.%d: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		_, hasValue := get(designPropertyEntries(t, d)[0], "Value")
		_, hasEditable := get(find(d, "Forms$DataView"), "Editable")
		_, hasPlaceholder := get(find(d, "Forms$TextBox"), "Placeholder")
		if hasValue == tt.flatDesignProps || hasEditable != tt.editable || hasPlaceholder != tt.placeholder {
			t.Errorf("%d.%d: design property Value %v, data view Editable %v, Placeholder %v; want flat design properties %v, Editable %v, Placeholder %v",
				tt.v.MajorVersion, tt.v.MinorVersion, hasValue, hasEditable, hasPlaceholder, tt.flatDesignProps, tt.editable, tt.placeholder)
		}
	}
}

// What the older shape cannot hold is refused, naming it — never dropped.
func TestNormalize_RefusesWhatTheOlderShapeCannotHold(t *testing.T) {
	compound := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$CompoundDesignPropertyValue"},
		{Key: "Properties", Value: bson.A{int32(2), designProperty("margin-top", toggleValue())}}}
	param := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$ClientTemplateParameter"}, {Key: "Expression", Value: "'x'"}}
	for _, tt := range []struct {
		name string
		v    *version.ProjectVersion
		doc  bson.D
		want string
	}{
		{"a Spacing (compound) design property on 9.24", pv(9, 24, 30),
			shapeSnippet("Always", bson.A{int32(3), designProperty("Spacing", compound)}, clientTemplate("")), "Spacing"},
		{"conditional data view editability on 10.6", pv(10, 6, 0),
			shapeSnippet("Conditional", bson.A{int32(3)}, clientTemplate("")), "Conditional"},
		{"a placeholder with parameters on 10.10", pv(10, 10, 0),
			shapeSnippet("Always", bson.A{int32(3)}, clientTemplate("Hi {1}", param)), "placeholder"},
	} {
		_, err := normalizeShape(t, tt.v, tt.doc)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want a refusal naming %q", tt.name, err, tt.want)
		}
	}
}

// A data view on a page parameter: below 10.0 the source names the parameter
// itself — PageParameter / SnippetParameter strings, as Studio Pro 9.24.30
// stores them — and SourceVariable (declared on EntityPathSource from 10.0) is
// unknown, so 9.24 saw no parameter at all: CE6050 "The selected entity must
// be configured with a page parameter". ForceFullObjects (Forms$DataSource,
// 10.8) is likewise a supertype key the floor never saw.
func dataViewOnParameter(pageParam, snippetParam, widget string, useAllPages bool) bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$DataView"},
		{Key: "DataSource", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: nil},
			{Key: "ForceFullObjects", Value: false},
			{Key: "SourceVariable", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$PageVariable"},
				{Key: "PageParameter", Value: pageParam}, {Key: "SnippetParameter", Value: snippetParam},
				{Key: "UseAllPages", Value: useAllPages}, {Key: "Widget", Value: widget}}}}},
		{Key: "Editability", Value: "Always"},
		{Key: "Name", Value: "dv"}}
}

func TestNormalize_DataViewSourceInTheMendix9Shape(t *testing.T) {
	doc := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Snippet"}, {Key: "Name", Value: "S"},
		{Key: "Widgets", Value: bson.A{int32(2), dataViewOnParameter("Contact", "", "", false)}}}
	d, err := normalizeShape(t, pv(9, 24, 30), doc)
	if err != nil {
		t.Fatal(err)
	}
	src := find(d, "Forms$DataViewSource")
	if got := strings.Join(keysOf(src), ","); got != "$ID,$Type,EntityRef,PageParameter,SnippetParameter" {
		t.Errorf("9.24 data view source keys = %s, want $ID,$Type,EntityRef,PageParameter,SnippetParameter", got)
	}
	if p, _ := get(src, "PageParameter"); p != "Contact" {
		t.Errorf("PageParameter = %v, want Contact", p)
	}

	// Control: 10.24 keeps SourceVariable, and ForceFullObjects from 10.8.
	d, err = normalizeShape(t, pv(10, 24, 24), bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Snippet"},
		{Key: "Name", Value: "S"}, {Key: "Widgets", Value: bson.A{int32(2), dataViewOnParameter("Contact", "", "", false)}}})
	if err != nil {
		t.Fatal(err)
	}
	src = find(d, "Forms$DataViewSource")
	for _, k := range []string{"SourceVariable", "ForceFullObjects"} {
		if _, ok := get(src, k); !ok {
			t.Errorf("10.24 lost %s: %v", k, keysOf(src))
		}
	}
	if _, ok := get(src, "PageParameter"); ok {
		t.Errorf("10.24 data view source got a PageParameter")
	}
}

// A source the older shape cannot name — another widget's object, every page's
// — is refused.
func TestNormalize_RefusesADataViewSourceMendix9CannotName(t *testing.T) {
	for _, dv := range []bson.D{
		dataViewOnParameter("", "", "dvOuter", false),
		dataViewOnParameter("", "", "", true),
	} {
		doc := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Snippet"}, {Key: "Name", Value: "S"},
			{Key: "Widgets", Value: bson.A{int32(2), dv}}}
		if _, err := normalizeShape(t, pv(9, 24, 30), doc); err == nil || !strings.Contains(err.Error(), "dv") {
			t.Errorf("a data view source 9.24 cannot name was not refused naming the data view: %v", err)
		}
	}
}
