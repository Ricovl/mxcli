// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// PROPOSAL_alter_object_list_item_address.md (ako/mxcli#1326,
// mendixlabs/mxcli#1453): an Accordion group stores no Name, so
// `insert into grp1 { … }` never resolved. It is addressed as an item of the
// accordion — `acc1 item@1`, `acc1 item('One')`, `acc1 item(btnIn)` — and a
// slot of it as `acc1 item@1.headercontent`.

func tpID(b byte) bson.Binary {
	return bson.Binary{Subtype: 0x04, Data: []byte{b, b, b, b, b, b, b, b, b, b, b, b, b, b, b, b}}
}

var (
	tpGroups     = tpID(0x10)
	tpRenderMode = tpID(0x11)
	tpHeaderText = tpID(0x12)
	tpHeaderSlot = tpID(0x13)
	tpContent    = tpID(0x14)
)

func textTemplate(text string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$ClientTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Template", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: text},
			}}},
		}},
	}
}

func namedWidget(name string) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: name}}
}

func widgetList(names ...string) bson.A {
	out := bson.A{int32(2)}
	for _, n := range names {
		out = append(out, namedWidget(n))
	}
	return out
}

// accordionGroup is one CustomWidgets$WidgetObject of the `groups` list.
func accordionGroup(header string, headerWidgets, bodyWidgets []string) bson.D {
	prop := func(tp bson.Binary, value bson.D) bson.D {
		return bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetProperty"}, {Key: "TypePointer", Value: tp}, {Key: "Value", Value: value}}
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: bson.A{int32(2),
			prop(tpRenderMode, bson.D{{Key: "PrimitiveValue", Value: "custom"}, {Key: "Widgets", Value: bson.A{int32(2)}}}),
			prop(tpHeaderText, bson.D{{Key: "TextTemplate", Value: textTemplate(header)}, {Key: "Widgets", Value: bson.A{int32(2)}}}),
			prop(tpHeaderSlot, bson.D{{Key: "Widgets", Value: widgetList(headerWidgets...)}}),
			prop(tpContent, bson.D{{Key: "Widgets", Value: widgetList(bodyWidgets...)}}),
		}},
	}
}

// accordionWithGroups is acc1 with the given groups, its schema declaring
// headerContent BEFORE content — so the body is chosen by name, not position.
func accordionWithGroups(groups ...bson.D) bson.D {
	pt := func(id bson.Binary, key, kind string) bson.D {
		return bson.D{{Key: "$ID", Value: id}, {Key: "PropertyKey", Value: key}, {Key: "ValueType", Value: bson.D{{Key: "Type", Value: kind}}}}
	}
	objects := bson.A{int32(2)}
	for _, g := range groups {
		objects = append(objects, g)
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "acc1"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "WidgetId", Value: "com.mendix.widget.web.accordion.Accordion"},
			{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: tpGroups},
				{Key: "PropertyKey", Value: "groups"},
				{Key: "ValueType", Value: bson.D{
					{Key: "Type", Value: "Object"},
					{Key: "IsList", Value: true},
					{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2),
						pt(tpRenderMode, "headerRenderMode", "Enumeration"),
						pt(tpHeaderText, "headerText", "TextTemplate"),
						pt(tpHeaderSlot, "headerContent", "Widgets"),
						pt(tpContent, "content", "Widgets"),
					}}}},
				}},
			}}}}},
		}},
		{Key: "Object", Value: bson.D{{Key: "Properties", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: tpGroups},
			{Key: "Value", Value: bson.D{{Key: "Objects", Value: objects}, {Key: "Widgets", Value: bson.A{int32(2)}}}},
		}}}}},
	}
}

func twoGroupMutator() *Mutator {
	return New(makeRawPage(accordionWithGroups(
		accordionGroup("One", nil, []string{"btnIn"}),
		accordionGroup("Two", nil, nil),
	)), model.ID("u"), &stubWidgetDeps{})
}

// groupsOf returns acc1's stored groups.
func groupsOf(t *testing.T, m *Mutator) []any {
	t.Helper()
	acc := findWidgetForTest(t, m.rawData, "acc1")
	prop := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(acc, "Object"), "Properties"))[0].(bson.D)
	return bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(prop, "Value"), "Objects"))
}

// slotNames returns the widget names in group g's property tp, checking the
// list keeps its [2] marker.
func slotNames(t *testing.T, m *Mutator, g int, tp bson.Binary) []string {
	t.Helper()
	group := groupsOf(t, m)[g].(bson.D)
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(group, "Properties")) {
		pd := p.(bson.D)
		if bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer")) != bsonnav.ExtractBinaryIDFromDoc(tp) {
			continue
		}
		raw := bsonnav.ToBsonA(bsonnav.DGet(bsonnav.DGetDoc(pd, "Value"), "Widgets"))
		if len(raw) == 0 || raw[0] != int32(2) {
			t.Fatalf("slot lost its [2] marker: %v", raw)
		}
		var names []string
		for _, w := range raw[1:] {
			names = append(names, bsonnav.DGetString(w.(bson.D), "Name"))
		}
		return names
	}
	t.Fatalf("group %d has no such property", g)
	return nil
}

func itemButton(name string) []pages.Widget {
	b := &pages.ActionButton{}
	b.Name = name
	return []pages.Widget{b}
}

var into = backend.InsertPosition("into")

func TestInsertIntoItemBody_EverySelector(t *testing.T) {
	for _, c := range []struct {
		ref   string
		group int
		want  string
	}{
		{"acc1 item@1", 0, "btnIn,btnNew"},
		{"acc1 item@2", 1, "btnNew"}, // an EMPTY body — the case nothing could reach
		{"acc1 item('Two')", 1, "btnNew"},
		{"acc1 item(btnIn)", 0, "btnIn,btnNew"},
	} {
		m := twoGroupMutator()
		if err := m.InsertWidget(c.ref, "", into, itemButton("btnNew")); err != nil {
			t.Fatalf("insert into %s: %v", c.ref, err)
		}
		if got := strings.Join(slotNames(t, m, c.group, tpContent), ","); got != c.want {
			t.Errorf("insert into %s: group %d content = %s, want %s", c.ref, c.group+1, got, c.want)
		}
		if got := slotNames(t, m, c.group, tpHeaderSlot); len(got) != 0 {
			t.Errorf("insert into %s wrote into headerContent too: %v", c.ref, got)
		}
	}
}

func TestInsertIntoItemSlot(t *testing.T) {
	m := twoGroupMutator()
	if err := m.InsertWidget("acc1 item@1", "headercontent", into, itemButton("btnHead")); err != nil {
		t.Fatalf("insert into acc1 item@1.headercontent: %v", err)
	}
	if got := slotNames(t, m, 0, tpHeaderSlot); len(got) != 1 || got[0] != "btnHead" {
		t.Errorf("headerContent = %v, want [btnHead]", got)
	}
	if got := strings.Join(slotNames(t, m, 0, tpContent), ","); got != "btnIn" {
		t.Errorf("content = %s, want btnIn untouched", got)
	}
}

func TestReplaceAndDropItemSlot(t *testing.T) {
	m := twoGroupMutator()
	if err := m.ReplaceWidget("acc1 item@1", "content", itemButton("btnRep")); err != nil {
		t.Fatalf("replace acc1 item@1.content: %v", err)
	}
	if got := strings.Join(slotNames(t, m, 0, tpContent), ","); got != "btnRep" {
		t.Errorf("after replace content = %s, want btnRep", got)
	}
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "acc1 item@1", Column: "content"}}); err != nil {
		t.Fatalf("drop acc1 item@1.content: %v", err)
	}
	if got := slotNames(t, m, 0, tpContent); len(got) != 0 {
		t.Errorf("after drop content = %v, want empty", got)
	}
	if n := len(groupsOf(t, m)); n != 2 {
		t.Errorf("dropping a slot removed a group: %d groups left", n)
	}
}

func TestDropItem(t *testing.T) {
	m := twoGroupMutator()
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "acc1 item@2"}}); err != nil {
		t.Fatalf("drop acc1 item@2: %v", err)
	}
	groups := groupsOf(t, m)
	if len(groups) != 1 {
		t.Fatalf("%d groups left, want 1", len(groups))
	}
	if got := strings.Join(slotNames(t, m, 0, tpContent), ","); got != "btnIn" {
		t.Errorf("the wrong group was dropped: remaining content %s", got)
	}
}

func TestSetItemProperty(t *testing.T) {
	m := twoGroupMutator()
	if err := m.SetWidgetProperty("acc1 item@1", "headerText", "Renamed"); err != nil {
		t.Fatalf("set headerText on acc1 item@1: %v", err)
	}
	if err := m.InsertWidget("acc1 item('Renamed')", "", into, itemButton("btnNew")); err != nil {
		t.Fatalf("the new header text does not address the group: %v", err)
	}
}

func TestResolveAlterTargetItem(t *testing.T) {
	m := twoGroupMutator()
	for _, c := range []struct {
		path []string
		kind string
	}{
		{[]string{"acc1 item@1"}, "item"},
		{[]string{"acc1 item@1", "headercontent"}, "region"},
	} {
		got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: c.path})
		if err != nil {
			t.Fatalf("ResolveAlterTarget(%v): %v", c.path, err)
		}
		if got.Kind != c.kind {
			t.Errorf("ResolveAlterTarget(%v) kind = %q, want %q", c.path, got.Kind, c.kind)
		}
	}
}

func TestItemAddressMissAndAmbiguity(t *testing.T) {
	m := New(makeRawPage(accordionWithGroups(
		accordionGroup("Same", nil, nil),
		accordionGroup("Same", nil, nil),
	)), model.ID("u"), &stubWidgetDeps{})
	err := m.InsertWidget("acc1 item('Same')", "", into, itemButton("b"))
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "@1") {
		t.Errorf("two groups titled Same: want an ambiguity error naming @n, got %v", err)
	}
	err = m.InsertWidget("acc1 item@3", "", into, itemButton("b"))
	if err == nil || !strings.Contains(err.Error(), "item@1") || !strings.Contains(err.Error(), "item@2") {
		t.Errorf("item@3 of two: want a miss listing the addresses that resolve, got %v", err)
	}
	if err := m.InsertWidget("acc1 item('Same')@2", "", into, itemButton("b")); err != nil {
		t.Errorf("@n among matches: %v", err)
	}
}

// Adding a new group is not part of this change; BEFORE/AFTER an item must be
// refused, not treated as INTO or written into the Objects list as a widget.
func TestInsertBesideItemIsRefused(t *testing.T) {
	m := twoGroupMutator()
	err := m.InsertWidget("acc1 item@1", "", backend.InsertPosition("after"), itemButton("b"))
	if err == nil {
		t.Fatal("insert after acc1 item@1 { widget } must be refused")
	}
	if n := len(groupsOf(t, m)); n != 2 {
		t.Errorf("a refused insert changed the group count to %d", n)
	}
}
