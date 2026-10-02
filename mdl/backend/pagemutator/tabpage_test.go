// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// translation builds a Texts$Translation as Studio Pro stores one.
func translation(lang, text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: "t-" + lang},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
}

// textsText builds a Texts$Text holding the given translations.
func textsText(items ...bson.D) bson.D {
	arr := bson.A{int32(3)}
	for _, it := range items {
		arr = append(arr, it)
	}
	return bson.D{
		{Key: "$ID", Value: "text"},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: arr},
	}
}

// makeTabControlPage mirrors Administration.Account_Overview as Studio Pro
// authors it: a Forms$TabControl whose Forms$TabPage carries its own Name and a
// Texts$Text Caption with an en_US translation only, and a child widget.
func makeTabControlPage() bson.D {
	tabPageID := bsonutil.NewIDBsonBinary()
	tabPage := bson.D{
		{Key: "$ID", Value: tabPageID},
		{Key: "$Type", Value: "Forms$TabPage"},
		{Key: "Badge", Value: nil},
		{Key: "Caption", Value: textsText(translation("en_US", "Local Users"))},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: "tabPage2"},
		{Key: "RefreshOnShow", Value: false},
		{Key: "Widgets", Value: bson.A{int32(2), bson.D{
			{Key: "$Type", Value: "Forms$DivContainer"},
			{Key: "Name", Value: "inner"},
			{Key: "Widgets", Value: bson.A{int32(2)}},
		}}},
	}
	tabControl := bson.D{
		{Key: "$Type", Value: "Forms$TabControl"},
		{Key: "Name", Value: "tabControl"},
		{Key: "DefaultPagePointer", Value: tabPageID},
		{Key: "TabPages", Value: bson.A{int32(3), tabPage}},
	}
	return makeRawPage(tabControl)
}

// A tab page is addressed by its own Name. findInWidgetChildren descended into
// TabPages[].Widgets but never matched the TabPage itself, so `set … on
// tabPage2` failed with `widget "tabPage2" not found` — the only fix for a
// missing tab caption translation (mxbuild CE4899) was re-creating the page.
func TestTabPage_IsAddressable(t *testing.T) {
	raw := makeTabControlPage()
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}

	r := findBsonWidget(raw, "tabPage2")
	if r == nil {
		t.Fatal(`findBsonWidget("tabPage2") = nil; a tab page must resolve by its Name`)
	}
	if typ := bsonnav.DGetString(r.widget, "$Type"); typ != "Forms$TabPage" {
		t.Fatalf("resolved to %s, want Forms$TabPage", typ)
	}
	if findBsonWidget(raw, "inner") == nil {
		t.Fatal("a widget inside the tab page no longer resolves")
	}
	if _, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"tabPage2"}}); err != nil {
		t.Fatalf("ResolveAlterTarget(tabPage2): %v", err)
	}
	if _, ok := m.WidgetScope()["tabPage2"]; !ok {
		t.Error("WidgetScope lacks tabPage2: duplicate-name checks would not see a tab page")
	}
	if _, found := findNearestDataSourceDoc(raw, "tabPage2"); !found {
		t.Error("findNearestDataSourceDoc does not find tabPage2")
	}
}

// Visibility is the other property a tab page carries; it goes through the
// common ConditionalVisibilitySettings path once the tab page resolves.
func TestTabPage_SetVisible(t *testing.T) {
	raw := makeTabControlPage()
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("tabPage2", "VisibleIf", "$currentUser != empty"); err != nil {
		t.Fatalf("set VisibleIf on tabPage2: %v", err)
	}
	cvs := bsonnav.DGetDoc(findBsonWidget(raw, "tabPage2").widget, "ConditionalVisibilitySettings")
	if bsonnav.DGetString(cvs, "Expression") != "$currentUser != empty" {
		t.Fatalf("ConditionalVisibilitySettings = %v", cvs)
	}
}

// INSERT INTO a tab page appends to its Widgets — the tab page is a simple
// container. INSERT BEFORE/AFTER, REPLACE and DROP would write widgets into (or
// remove entries from) the tab control's TabPages list, which holds only tab
// pages and which the control's DefaultPagePointer points into: refused.
func TestTabPage_StructuralOps(t *testing.T) {
	newWidget := textBoxes("added")

	m := New(makeTabControlPage(), model.ID("page-1"), &stubWidgetDeps{})
	if err := m.InsertWidget("tabPage2", "", "into", newWidget); err != nil {
		t.Fatalf("insert into tabPage2: %v", err)
	}
	if r := findBsonWidget(m.rawData, "added"); r == nil || r.parentKey != "Widgets" {
		t.Fatal("inserted widget is not in the tab page's Widgets")
	}

	for name, op := range map[string]func(m *Mutator) error{
		"insert after":  func(m *Mutator) error { return m.InsertWidget("tabPage2", "", "after", newWidget) },
		"insert before": func(m *Mutator) error { return m.InsertWidget("tabPage2", "", "before", newWidget) },
		"replace":       func(m *Mutator) error { return m.ReplaceWidget("tabPage2", "", newWidget) },
		"drop":          func(m *Mutator) error { return m.DropWidget([]backend.WidgetRef{{Widget: "tabPage2"}}) },
	} {
		t.Run(name, func(t *testing.T) {
			m := New(makeTabControlPage(), model.ID("page-1"), &stubWidgetDeps{})
			err := op(m)
			if err == nil || !strings.Contains(err.Error(), "tab page") {
				t.Fatalf("%s on a tab page: err = %v, want a refusal naming the tab page", name, err)
			}
			tabs := bsonnav.DGetArrayElements(bsonnav.DGet(findBsonWidget(m.rawData, "tabControl").widget, "TabPages"))
			if len(tabs) != 1 {
				t.Fatalf("TabPages changed to %d entries by a refused %s", len(tabs), name)
			}
		})
	}
}
