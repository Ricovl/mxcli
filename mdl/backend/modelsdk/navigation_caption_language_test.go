// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// testCaption builds a Texts$Text with one translation per (language, text)
// pair, in the order given.
func testCaption(pairs ...string) bson.D {
	items := bson.A{int32(3)}
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: pairs[i]},
			{Key: "Text", Value: pairs[i+1]},
		})
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: items},
	}
}

// testMenuItem builds a Studio Pro-shaped Menus$MenuItem with a microflow
// action (not a page, so the reader's old "no caption and no page" filter
// applies to it) and the given sub-items.
func testMenuItem(caption bson.D, subs ...bson.D) bson.D {
	items := bson.A{int32(3)}
	for _, s := range subs {
		items = append(items, s)
	}
	action := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$MicroflowAction"},
		{Key: "DisabledDuringExecution", Value: false},
		{Key: "MicroflowSettings", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$MicroflowSettings"},
			{Key: "Microflow", Value: "MyFirstModule.ACT_Something"},
		}},
	}
	if len(subs) > 0 {
		action = bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Forms$NoAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: action},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: caption},
		{Key: "Icon", Value: nil},
		{Key: "Items", Value: items},
	}
}

// storeProfileMenu replaces the first web profile's menu with items, written
// as a patch so nothing is re-paired or carried, and returns the profile name.
func storeProfileMenu(t *testing.T, b *Backend, items ...bson.D) string {
	t.Helper()
	nav, err := b.GetNavigation()
	if err != nil || nav == nil {
		t.Fatalf("GetNavigation: %v", err)
	}
	raw, err := b.reader.GetRawUnitBytes(string(nav.ID))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	profiles := navGetArray(doc, "Profiles")
	name := ""
	for i, p := range profiles {
		prof, ok := p.(bson.D)
		if !ok || navGetString(prof, "$Type") != "Navigation$NavigationProfile" {
			continue
		}
		coll := bson.A{int32(3)}
		for _, it := range items {
			coll = append(coll, it)
		}
		prof = navSetField(prof, "Menu", bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: coll},
		})
		profiles[i] = prof
		name = navGetString(prof, "Name")
		break
	}
	if name == "" {
		t.Fatal("fixture has no web profile")
	}
	doc = navSetField(doc, "Profiles", profiles)
	out, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnitPatch(string(nav.ID), out); err != nil {
		t.Fatalf("write: %v", err)
	}
	return name
}

// A navigation menu item's caption is read with the rule a page's texts are
// (model.SelectTranslation): the project's default language, an empty entry
// in it included, else en_US, else the first non-empty translation. The reader
// used to take the first non-empty translation in document order, so on a
// project whose default language is nl_NL it printed the en_US caption a page
// describe would not, and an alter address copied from a page-style read would
// not have matched.
//
// It also keeps an item whose caption reads empty and whose action is not a
// page. The reader dropped one, so describe never printed it and a describe ->
// exec round trip deleted it.
func TestGetNavigation_CaptionUsesDefaultLanguage(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	t.Cleanup(func() { model.SetAuthoringLanguage("") })

	profile := storeProfileMenu(t, b,
		testMenuItem(testCaption("en_US", "Home", "nl_NL", "Start")),
		testMenuItem(testCaption("en_US", "Docs")),
		testMenuItem(testCaption("nl_NL", "", "en_US", "Icon only")),
	)

	model.SetAuthoringLanguage("nl_NL")
	nav, err := b.GetNavigation()
	if err != nil {
		t.Fatalf("GetNavigation: %v", err)
	}
	var got []string
	for _, p := range nav.Profiles {
		if p.Name != profile {
			continue
		}
		for _, it := range p.MenuItems {
			got = append(got, it.Caption)
		}
	}
	want := []string{"Start", "Docs", ""}
	if len(got) != len(want) {
		t.Fatalf("captions = %q, want %q (an item whose caption reads empty must not be dropped)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("caption %d = %q, want %q (default language nl_NL, then en_US, then first non-empty)", i, got[i], want[i])
		}
	}
}
