// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// spliceFixture is a fixture copy whose first web profile holds a
// Studio Pro-shaped nested, two-language menu:
//
//	'Home'                 en_US Home, nl_NL Start
//	'Admin' { 'Users', 'Old import' }
//	'Reports' { 'Users' }
//	'Help'
func spliceFixture(t *testing.T) (*Backend, string) {
	t.Helper()
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
	profile := storeProfileMenu(t, b,
		testMenuItem(testCaption("en_US", "Home", "nl_NL", "Start")),
		testMenuItem(testCaption("en_US", "Admin"),
			testMenuItem(testCaption("en_US", "Users", "nl_NL", "Gebruikers")),
			testMenuItem(testCaption("en_US", "Old import"))),
		testMenuItem(testCaption("en_US", "Reports"),
			testMenuItem(testCaption("en_US", "Users"))),
		testMenuItem(testCaption("en_US", "Help")),
	)
	return b, profile
}

// storedMenu reads the profile's Menus$MenuItem documents back from storage,
// keyed by $ID, each with its encoded bytes, and the caption tree.
func storedMenu(t *testing.T, b *Backend, profile string) (map[string][]byte, []*backend.MenuTreeNode) {
	t.Helper()
	m, err := b.OpenNavigationProfileForMutation(profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	nav, _ := b.GetNavigation()
	doc, err := b.rawUnitDoc(nav.ID)
	if err != nil {
		t.Fatal(err)
	}
	items := map[string][]byte{}
	var walk func(v any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if navGetString(d, "$Type") == "Menus$MenuItem" {
				raw, err := bson.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				items[idHex(d)] = raw
			}
			for _, e := range d {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	for _, p := range navGetArray(doc, "Profiles") {
		if pd, ok := p.(bson.D); ok && strings.EqualFold(navGetString(pd, "Name"), profile) {
			walk(navGetDoc(pd, "Menu"))
		}
	}
	return items, m.MenuTree()
}

func idHex(d bson.D) string {
	for _, e := range d {
		if e.Key == "$ID" {
			if bin, ok := e.Value.(primitive.Binary); ok {
				return fmt.Sprintf("%x", bin.Data)
			}
		}
	}
	return ""
}

// survivorsChanged lists every item of before that is still stored but no
// longer byte-identical — the splice's promise for items no operation names.
// Items whose subtree an operation changed are listed in touched and skipped.
func survivorsChanged(before, after map[string][]byte, touched map[string]bool) []string {
	var out []string
	for id, raw := range before {
		if touched[id] {
			continue
		}
		got, ok := after[id]
		if !ok {
			out = append(out, id+" missing")
			continue
		}
		if !bytes.Equal(got, raw) {
			out = append(out, id+" re-encoded")
		}
	}
	return out
}

func captionsOf(tree []*backend.MenuTreeNode) string {
	var parts []string
	for _, n := range tree {
		s := "'" + n.Caption + "'"
		if len(n.Items) > 0 {
			s += "{" + captionsOf(n.Items) + "}"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func resolve(t *testing.T, tree []*backend.MenuTreeNode, segs ...string) backend.MenuItemPath {
	t.Helper()
	var a types.MenuItemAddress
	for _, s := range segs {
		a.Segments = append(a.Segments, types.MenuItemAddressSegment{Caption: s})
	}
	p, err := backend.ResolveMenuItemAddress(a, tree)
	if err != nil {
		t.Fatalf("resolve %s: %v", a, err)
	}
	return p
}

// idOfPath is the stored $ID of the item at path.
func idOfPath(t *testing.T, b *Backend, profile string, path backend.MenuItemPath) string {
	t.Helper()
	nav, _ := b.GetNavigation()
	doc, _ := b.rawUnitDoc(nav.ID)
	for _, p := range navGetArray(doc, "Profiles") {
		pd, ok := p.(bson.D)
		if !ok || !strings.EqualFold(navGetString(pd, "Name"), profile) {
			continue
		}
		holder := navGetDoc(pd, "Menu")
		var it bson.D
		for _, i := range path {
			var els []bson.D
			for _, v := range navGetArray(holder, "Items") {
				if d, ok := v.(bson.D); ok {
					els = append(els, d)
				}
			}
			it = els[i]
			holder = it
		}
		return idHex(it)
	}
	return ""
}

// Dropping an item keeps every other item byte-identical, $ID included.
//
// The control is the rebuild the splice replaces: the same post-drop menu
// written by navMenuItemBson, as `create or modify navigation` writes it, and
// saved as a patch so canon does not re-pair the $IDs. It must fail the same
// assertion — otherwise the assertion could not tell a splice from a rebuild.
func TestMenuSplice_DropKeepsSurvivorsByteIdentical(t *testing.T) {
	b, profile := spliceFixture(t)
	before, tree := storedMenu(t, b, profile)
	dropped := idOfPath(t, b, profile, resolve(t, tree, "Old import"))
	admin := idOfPath(t, b, profile, resolve(t, tree, "Admin"))

	m, err := b.OpenNavigationProfileForMutation(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DropMenuItem(resolve(t, m.MenuTree(), "Old import")); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after, tree2 := storedMenu(t, b, profile)
	if got, want := captionsOf(tree2), "'Home' 'Admin'{'Users'} 'Reports'{'Users'} 'Help'"; got != want {
		t.Fatalf("menu after drop = %s, want %s", got, want)
	}
	if _, still := after[dropped]; still {
		t.Error("the dropped item is still stored")
	}
	touched := map[string]bool{dropped: true, admin: true} // Admin's Items list changed
	if bad := survivorsChanged(before, after, touched); len(bad) > 0 {
		t.Errorf("items the drop did not name changed: %v", bad)
	}

	// Control: the rebuild.
	b2, profile2 := spliceFixture(t)
	before2, _ := storedMenu(t, b2, profile2)
	nav, _ := b2.GetNavigation()
	if err := b2.UpdateNavigationProfile(nav.ID, profile2, types.NavigationProfileSpec{HasMenu: true, MenuItems: []types.NavMenuItemSpec{
		{Caption: "Home", Microflow: "MyFirstModule.ACT_Something"},
		{Caption: "Admin", Items: []types.NavMenuItemSpec{{Caption: "Users", Microflow: "MyFirstModule.ACT_Something"}}},
		{Caption: "Reports", Items: []types.NavMenuItemSpec{{Caption: "Users", Microflow: "MyFirstModule.ACT_Something"}}},
		{Caption: "Help", Microflow: "MyFirstModule.ACT_Something"},
	}}); err != nil {
		t.Fatal(err)
	}
	after2, _ := storedMenu(t, b2, profile2)
	if bad := survivorsChanged(before2, after2, map[string]bool{}); len(bad) == 0 {
		t.Error("control: a rebuilt menu passed the byte-identity assertion, so it cannot detect a rebuild")
	}
}

func TestMenuSplice_InsertPositions(t *testing.T) {
	b, profile := spliceFixture(t)
	before, _ := storedMenu(t, b, profile)
	m, err := b.OpenNavigationProfileForMutation(profile)
	if err != nil {
		t.Fatal(err)
	}
	item := func(c string) []types.NavMenuItemSpec {
		return []types.NavMenuItemSpec{{Caption: c, Page: "MyFirstModule.Home"}}
	}
	tree := m.MenuTree()
	home := resolve(t, tree, "Home")
	steps := []struct {
		name string
		do   func() error
	}{
		{"after", func() error { return m.InsertMenuItems(home.Parent(), home.Index()+1, item("Orders")) }},
		{"before", func() error {
			p := resolve(t, m.MenuTree(), "Help")
			return m.InsertMenuItems(p.Parent(), p.Index(), item("FAQ"))
		}},
		{"into", func() error {
			p := resolve(t, m.MenuTree(), "Admin")
			return m.InsertMenuItems(p, len(backend.MenuNodeAt(m.MenuTree(), p).Items), item("Audit log"))
		}},
		{"top level", func() error { return m.InsertMenuItems(nil, len(m.MenuTree()), item("Last")) }},
		{"nested fragment", func() error {
			return m.InsertMenuItems(nil, 0, []types.NavMenuItemSpec{{Caption: "Group", Items: item("Child")}})
		}},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after, tree2 := storedMenu(t, b, profile)
	want := "'Group'{'Child'} 'Home' 'Orders' 'Admin'{'Users' 'Old import' 'Audit log'} 'Reports'{'Users'} 'FAQ' 'Help' 'Last'"
	if got := captionsOf(tree2); got != want {
		t.Fatalf("menu = %s\nwant   %s", got, want)
	}
	adminID := idOfPath(t, b, profile, resolve(t, tree2, "Admin"))
	if bad := survivorsChanged(before, after, map[string]bool{adminID: true}); len(bad) > 0 {
		t.Errorf("items no insert named changed: %v", bad)
	}

	// The inserted item carries Studio Pro's property set, and the sub-menu of
	// the nested fragment stores Forms$NoAction (CE0548 otherwise).
	nav, _ := b.GetNavigation()
	doc, _ := b.rawUnitDoc(nav.ID)
	raw, _ := bson.Marshal(doc)
	var group bson.D
	var find func(v any)
	find = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if navGetString(d, "$Type") == "Menus$MenuItem" && group == nil {
				for _, tr := range navGetArray(navGetDoc(d, "Caption"), "Items") {
					if td, ok := tr.(bson.D); ok && navGetString(td, "Text") == "Group" {
						group = d
					}
				}
			}
			for _, e := range d {
				find(e.Value)
			}
		case bson.A:
			for _, e := range d {
				find(e)
			}
		}
	}
	_ = raw
	find(doc)
	if group == nil {
		t.Fatal("inserted group not found")
	}
	var keys []string
	for _, e := range group {
		keys = append(keys, e.Key)
	}
	if want := []string{"$ID", "$Type", "Action", "AlternativeText", "Caption", "Icon", "Items"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("inserted item keys = %v, want Studio Pro's %v", keys, want)
	}
	if got := navGetString(navGetDoc(group, "Action"), "$Type"); got != "Forms$NoAction" {
		t.Errorf("sub-menu action = %s, want Forms$NoAction", got)
	}
}

// A move relocates the stored item itself: same bytes, $ID, sub-items and
// translations. A move into its own subtree is refused.
func TestMenuSplice_MoveKeepsTheItemsBytes(t *testing.T) {
	b, profile := spliceFixture(t)
	before, tree := storedMenu(t, b, profile)
	users := resolve(t, tree, "Admin", "Users")
	usersID := idOfPath(t, b, profile, users)
	adminID := idOfPath(t, b, profile, resolve(t, tree, "Admin"))
	helpPath := resolve(t, tree, "Help")

	m, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m.MoveMenuItem(resolve(t, m.MenuTree(), "Admin"), resolve(t, m.MenuTree(), "Admin", "Users"), backend.MenuInto); err == nil {
		t.Error("moving Admin into its own sub-item was accepted")
	}
	if err := m.MoveMenuItem(users, helpPath, backend.MenuAfter); err != nil {
		t.Fatal(err)
	}
	if err := m.MoveMenuItem(resolve(t, m.MenuTree(), "Home"), resolve(t, m.MenuTree(), "Reports"), backend.MenuInto); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after, tree2 := storedMenu(t, b, profile)
	if got, want := captionsOf(tree2), "'Admin'{'Old import'} 'Reports'{'Users' 'Home'} 'Help' 'Users'"; got != want {
		t.Fatalf("menu = %s, want %s", got, want)
	}
	if !bytes.Equal(after[usersID], before[usersID]) {
		t.Error("the moved item's bytes changed (its $ID, caption translations or action)")
	}
	reportsID := idOfPath(t, b, profile, resolve(t, tree2, "Reports"))
	if bad := survivorsChanged(before, after, map[string]bool{adminID: true, reportsID: true}); len(bad) > 0 {
		t.Errorf("items changed: %v", bad)
	}
}

// set Caption writes the authoring language's translation and keeps the
// others, and the caption element keeps its $ID; Icon none stores null.
func TestMenuSplice_SetProperties(t *testing.T) {
	b, profile := spliceFixture(t)
	model.SetAuthoringLanguage("nl_NL")
	before, tree := storedMenu(t, b, profile)
	home := resolve(t, tree, "Start") // nl_NL is the default: describe prints Start
	homeID := idOfPath(t, b, profile, home)

	m, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m.SetMenuItemCaption(home, "Begin"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetMenuItemIcon(home, types.NavMenuItemSpec{IconKind: types.MenuIconGlyph, IconCode: 57377}); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	after, _ := storedMenu(t, b, profile)
	var oldDoc, newDoc bson.D
	_ = bson.Unmarshal(before[homeID], &oldDoc)
	_ = bson.Unmarshal(after[homeID], &newDoc)
	if newDoc == nil {
		t.Fatal("the item lost its $ID")
	}
	got := map[string]string{}
	for _, tr := range navGetArray(navGetDoc(newDoc, "Caption"), "Items") {
		if d, ok := tr.(bson.D); ok {
			got[navGetString(d, "LanguageCode")] = navGetString(d, "Text")
		}
	}
	if want := map[string]string{"en_US": "Home", "nl_NL": "Begin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("caption translations = %v, want %v", got, want)
	}
	if idHex(navGetDoc(newDoc, "Caption")) != idHex(navGetDoc(oldDoc, "Caption")) {
		t.Error("the caption element's $ID changed")
	}
	if got := navGetString(navGetDoc(newDoc, "Icon"), "$Type"); got != "Forms$GlyphIcon" {
		t.Errorf("icon = %s, want Forms$GlyphIcon", got)
	}
	if bad := survivorsChanged(before, after, map[string]bool{homeID: true}); len(bad) > 0 {
		t.Errorf("items changed: %v", bad)
	}

	m2, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m2.SetMenuItemIcon(resolve(t, m2.MenuTree(), "Begin"), types.NavMenuItemSpec{}); err != nil {
		t.Fatal(err)
	}
	_ = m2.Save()
	after2, _ := storedMenu(t, b, profile)
	var d2 bson.D
	_ = bson.Unmarshal(after2[homeID], &d2)
	for _, e := range d2 {
		if e.Key == "Icon" && e.Value != nil {
			t.Errorf("Icon none stored %v, want null", e.Value)
		}
	}
}

// replace gives the replacement fresh $IDs and carries the other-language
// translations of a caption it keeps.
func TestMenuSplice_ReplaceCarriesTranslations(t *testing.T) {
	b, profile := spliceFixture(t)
	before, tree := storedMenu(t, b, profile)
	homeID := idOfPath(t, b, profile, resolve(t, tree, "Home"))
	m, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m.ReplaceMenuItem(resolve(t, m.MenuTree(), "Home"), []types.NavMenuItemSpec{
		{Caption: "Home", Page: "MyFirstModule.Home"},
		{Caption: "Extra", Page: "MyFirstModule.Home"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = m.Save()
	after, tree2 := storedMenu(t, b, profile)
	if got := captionsOf(tree2); !strings.HasPrefix(got, "'Home' 'Extra' 'Admin'") {
		t.Fatalf("menu = %s", got)
	}
	if _, still := after[homeID]; still {
		t.Error("the replaced item kept its $ID; a replacement is new content")
	}
	newID := idOfPath(t, b, profile, resolve(t, tree2, "Home"))
	var d bson.D
	_ = bson.Unmarshal(after[newID], &d)
	langs := map[string]string{}
	for _, tr := range navGetArray(navGetDoc(d, "Caption"), "Items") {
		if td, ok := tr.(bson.D); ok {
			langs[navGetString(td, "LanguageCode")] = navGetString(td, "Text")
		}
	}
	if langs["nl_NL"] != "Start" || langs["en_US"] != "Home" {
		t.Errorf("replacement caption = %v, want en_US Home and the carried nl_NL Start", langs)
	}
	if bad := survivorsChanged(before, after, map[string]bool{homeID: true}); len(bad) > 0 {
		t.Errorf("items changed: %v", bad)
	}
}

// The page clauses edit the stored elements in place and clear the way Studio
// Pro does: Form "" for the login page, null for the not-found page.
func TestMenuSplice_PageClauses(t *testing.T) {
	b, profile := spliceFixture(t)
	nav, _ := b.GetNavigation()
	profileDoc := func() bson.D {
		doc, _ := b.rawUnitDoc(nav.ID)
		for _, p := range navGetArray(doc, "Profiles") {
			if pd, ok := p.(bson.D); ok && strings.EqualFold(navGetString(pd, "Name"), profile) {
				return pd
			}
		}
		return nil
	}
	homeID := idHex(navGetDoc(profileDoc(), "HomePage"))

	m, _ := b.OpenNavigationProfileForMutation(profile)
	steps := []error{
		m.SetHomePage(types.NavHomePageSpec{IsPage: true, Target: "MyFirstModule.Home"}),
		m.SetHomePage(types.NavHomePageSpec{IsPage: true, Target: "MyFirstModule.Home", ForRole: "Administrator"}),
		m.SetHomePage(types.NavHomePageSpec{IsPage: false, Target: "MyFirstModule.ACT", ForRole: "User"}),
		m.SetLoginPage("MyFirstModule.Login"),
		m.SetNotFoundPage("MyFirstModule.NotFound"),
	}
	for i, err := range steps {
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	_ = m.Save()
	p := profileDoc()
	hp := navGetDoc(p, "HomePage")
	if idHex(hp) != homeID || navGetString(hp, "Page") != "MyFirstModule.Home" {
		t.Errorf("HomePage = %v (want the stored element, $ID %s, updated in place)", hp, homeID)
	}
	var roles []string
	for _, v := range navGetArray(p, "HomeItems") {
		if d, ok := v.(bson.D); ok {
			roles = append(roles, navGetString(d, "UserRole")+"="+navGetString(d, "Page")+navGetString(d, "Microflow"))
		}
	}
	if want := []string{"Administrator=MyFirstModule.Home", "User=MyFirstModule.ACT"}; !reflect.DeepEqual(roles, want) {
		t.Errorf("HomeItems = %v, want %v", roles, want)
	}
	loginID := idHex(navGetDoc(p, "LoginPageSettings"))

	m2, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m2.DropRoleHomePage("NoSuchRole"); err == nil || !strings.Contains(err.Error(), "Administrator, User") {
		t.Errorf("dropping a role with no home page: %v", err)
	}
	for _, err := range []error{m2.DropRoleHomePage("administrator"), m2.SetLoginPage(""), m2.SetNotFoundPage("")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = m2.Save()
	p = profileDoc()
	ls := navGetDoc(p, "LoginPageSettings")
	if ls == nil || navGetString(ls, "Form") != "" || idHex(ls) != loginID {
		t.Errorf("LoginPageSettings after drop = %v, want the same element with Form \"\"", ls)
	}
	for _, e := range p {
		if e.Key == "NotFoundHomepage" && e.Value != nil {
			t.Errorf("NotFoundHomepage after drop = %v, want null", e.Value)
		}
	}
	var left []string
	for _, v := range navGetArray(p, "HomeItems") {
		if d, ok := v.(bson.D); ok {
			left = append(left, navGetString(d, "UserRole"))
		}
	}
	if !reflect.DeepEqual(left, []string{"User"}) {
		t.Errorf("HomeItems after drop = %v, want [User]", left)
	}
}

// An alter that restates what is stored writes nothing (ADR-0008); the write
// that changes something lands — the control that makes "nothing written"
// mean the elision, not a dead write path.
func TestMenuSplice_NoChangeIsElided(t *testing.T) {
	b, profile := spliceFixture(t)
	m, _ := b.OpenNavigationProfileForMutation(profile)
	if err := m.SetMenuItemCaption(resolve(t, m.MenuTree(), "Help"), "Help"); err != nil {
		t.Fatal(err)
	}
	s0 := b.WriteStats()
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	s1 := b.WriteStats()
	if s1.Written != s0.Written {
		t.Errorf("a set to the stored value was written (%d -> %d)", s0.Written, s1.Written)
	}
	m2, _ := b.OpenNavigationProfileForMutation(profile)
	_ = m2.SetMenuItemCaption(resolve(t, m2.MenuTree(), "Help"), "Help!")
	_ = m2.Save()
	if s2 := b.WriteStats(); s2.Written != s1.Written+1 {
		t.Errorf("control: a real change was not written (%d -> %d)", s1.Written, s2.Written)
	}
}

// A menu document's items are the same element under ItemCollection, so the
// same splice edits them.
func TestMenuSplice_MenuDocument(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("module: %v", err)
	}
	md := &types.MenuDocument{Name: "SpliceMenu", ContainerID: mod.ID, Items: []*types.NavMenuItem{
		{Caption: "One", Page: "MyFirstModule.Home"},
		{Caption: "Two", Page: "MyFirstModule.Home"},
	}}
	if err := b.CreateMenuDocument(md); err != nil {
		t.Fatal(err)
	}
	m, err := b.OpenMenuDocumentForMutation(md.ID)
	if err != nil {
		t.Fatal(err)
	}
	two := resolve(t, m.MenuTree(), "Two")
	if err := m.InsertMenuItems(nil, two.Index(), []types.NavMenuItemSpec{{Caption: "Between", Page: "MyFirstModule.Home"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.DropMenuItem(resolve(t, m.MenuTree(), "One")); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := b.GetMenuDocumentByQualifiedName("MyFirstModule", "SpliceMenu")
	if err != nil {
		t.Fatal(err)
	}
	var caps []string
	for _, it := range got.Items {
		caps = append(caps, it.Caption)
	}
	if !reflect.DeepEqual(caps, []string{"Between", "Two"}) {
		t.Errorf("menu document items = %v, want [Between Two]", caps)
	}
}
