// SPDX-License-Identifier: Apache-2.0

package menumutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
)

type fakeDeps struct{ saved []byte }

func (f *fakeDeps) BuildMenuItem(spec types.NavMenuItemSpec) (bson.D, error) {
	return item(spec.Caption), nil
}
func (f *fakeDeps) BuildMenuAction(types.NavMenuItemSpec) (bson.D, error) { return nil, nil }
func (f *fakeDeps) BuildMenuIcon(types.NavMenuItemSpec) any               { return nil }
func (f *fakeDeps) SaveUnit(_ string, b []byte) error                     { f.saved = b; return nil }

func item(caption string, subs ...bson.D) bson.D {
	items := bson.A{int32(3)}
	for _, s := range subs {
		items = append(items, s)
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: bson.D{{Key: "$Type", Value: "Forms$NoAction"}}},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3),
			bson.D{{Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: caption}}}}}},
		{Key: "Icon", Value: nil},
		{Key: "Items", Value: items},
	}
}

func navDoc(profiles ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, p := range profiles {
		arr = append(arr, p)
	}
	return bson.D{{Key: "$Type", Value: "Navigation$NavigationDocument"}, {Key: "Profiles", Value: arr}}
}

func TestNewProfile_Refusals(t *testing.T) {
	native := bson.D{{Key: "$Type", Value: "Navigation$NativeNavigationProfile"}, {Key: "Name", Value: "NativePhone"}}
	web := bson.D{{Key: "$Type", Value: "Navigation$NavigationProfile"}, {Key: "Name", Value: "Responsive"}, {Key: "Menu", Value: nil}}
	doc := navDoc(web, native)
	if _, err := NewProfile(doc, "u", "NativePhone", &fakeDeps{}); err == nil || !strings.Contains(err.Error(), "bottom bar") {
		t.Errorf("native profile: %v, want a refusal naming the bottom bar", err)
	}
	if _, err := NewProfile(doc, "u", "Tablet", &fakeDeps{}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing profile: %v", err)
	}
	// A web profile whose Menu is null gets a collection on the first insert,
	// shaped as Studio Pro writes one.
	m, err := NewProfile(doc, "u", "responsive", &fakeDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.InsertMenuItems(nil, 0, []types.NavMenuItemSpec{{Caption: "First"}}); err != nil {
		t.Fatal(err)
	}
	if tree := m.MenuTree(); len(tree) != 1 || tree[0].Caption != "First" {
		t.Errorf("tree after insert into an empty menu = %+v", tree)
	}
	coll := m.collection()
	if got := coll[1].Value; got != "Menus$MenuItemCollection" {
		t.Errorf("created collection $Type = %v", got)
	}
	if err := m.SetHomePage(types.NavHomePageSpec{}); err != nil {
		t.Errorf("SetHomePage on a profile: %v", err)
	}
}

func TestMenuDocument_HasNoPageClauses(t *testing.T) {
	doc := bson.D{{Key: "$Type", Value: "Menus$MenuDocument"}, {Key: "ItemCollection", Value: bson.D{
		{Key: "$Type", Value: "Menus$MenuItemCollection"}, {Key: "Items", Value: bson.A{int32(3), item("A", item("B"))}}}}}
	m, err := NewMenuDocument(doc, "u", &fakeDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetLoginPage("M.P"); err == nil {
		t.Error("a menu document accepted a login page")
	}
	a := backend.MenuItemPath{0}
	if err := m.MoveMenuItem(a, a, backend.MenuAfter); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Errorf("move relative to itself: %v", err)
	}
	if err := m.MoveMenuItem(a, backend.MenuItemPath{0, 0}, backend.MenuInto); err == nil || !strings.Contains(err.Error(), "own sub-items") {
		t.Errorf("move into own subtree: %v", err)
	}
	if _, err := NewMenuDocument(navDoc(), "u", &fakeDeps{}); err == nil {
		t.Error("a navigation document was opened as a menu document")
	}
}
