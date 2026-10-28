// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1756 (mendixlabs/mxcli#1664): a profile's application title and
// icon had no clause, so every generated app kept Studio Pro's "Mendix" in the
// browser tab. Studio Pro stores them as
//
//	AppTitle: { $Type: 'Texts$Text', Items: [3, { $Type: 'Texts$Translation', LanguageCode: 'en_US', Text: 'Mendix' }] }
//	AppIcon:  'Atlas_Core.Content.Mendix'

// appTitleStored is a Studio Pro-shaped AppTitle with fixed ids, so a test can
// tell a patched element from a rebuilt one.
func appTitleStored(pairs ...string) bson.D {
	items := bson.A{int32(3)}
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, bson.D{
			{Key: "$ID", Value: "item-" + pairs[i]},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: pairs[i]},
			{Key: "Text", Value: pairs[i+1]},
		})
	}
	return bson.D{{Key: "$ID", Value: "app-title"}, {Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: items}}
}

func appTitleProfile(title any, icon string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "AppIcon", Value: icon},
		{Key: "AppTitle", Value: title},
		{Key: "Kind", Value: "Responsive"},
		{Key: "Name", Value: "Responsive"},
	}
}

// translationsOf maps a stored AppTitle's items by language to "$ID=Text".
func translationsOf(t *testing.T, doc bson.D) (textID any, items map[string]string) {
	t.Helper()
	title := navGetDoc(doc, "AppTitle")
	if title == nil {
		t.Fatalf("AppTitle is %v, want a Texts$Text", navField(doc, "AppTitle"))
	}
	if navGetString(title, "$Type") != "Texts$Text" {
		t.Fatalf("AppTitle $Type = %q", navGetString(title, "$Type"))
	}
	items = map[string]string{}
	arr := navGetArray(title, "Items")
	if len(arr) == 0 || arr[0] != int32(3) {
		t.Errorf("AppTitle Items = %v, want the marker-3 list Studio Pro writes", arr)
	}
	for _, it := range arr {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		id := ""
		if s, ok := navField(d, "$ID").(string); ok {
			id = s
		} else {
			id = "(fresh)"
		}
		items[navGetString(d, "LanguageCode")] = id + "=" + navGetString(d, "Text")
	}
	return navField(title, "$ID"), items
}

func navField(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return "absent"
}

// The title is written as the project default language's item of the stored
// text, in place: its own $ID and every other language's item are kept, as
// Studio Pro keeps them when the title is edited in the default language.
func TestNavPatchWebProfile_AppTitleWritesTheDefaultLanguageItem(t *testing.T) {
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
	title := "EWO"
	for _, c := range []struct {
		lang string
		want map[string]string
	}{
		{"en_US", map[string]string{"en_US": "item-en_US=EWO", "nl_NL": "item-nl_NL=Mijn app"}},
		{"nl_NL", map[string]string{"en_US": "item-en_US=Mendix", "nl_NL": "item-nl_NL=EWO"}},
		// No item in the default language yet: one is added, nothing replaced.
		{"de_DE", map[string]string{"en_US": "item-en_US=Mendix", "nl_NL": "item-nl_NL=Mijn app", "de_DE": "(fresh)=EWO"}},
	} {
		model.SetAuthoringLanguage(c.lang)
		stored := appTitleProfile(appTitleStored("en_US", "Mendix", "nl_NL", "Mijn app"), "")
		before := mustMarshal(t, stored)
		out := mustNavPatchWebProfile(t, stored, types.NavigationProfileSpec{AppTitle: &title})
		id, got := translationsOf(t, out)
		if id != "app-title" {
			t.Errorf("%s: AppTitle $ID = %v, want the stored one kept", c.lang, id)
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: items %v, want %v", c.lang, got, c.want)
		}
		for lang, w := range c.want {
			if got[lang] != w {
				t.Errorf("%s: %s item = %q, want %q (all: %v)", c.lang, lang, got[lang], w, got)
			}
		}
		// The caller's stored profile is not changed under it.
		if !bytes.Equal(before, mustMarshal(t, stored)) {
			t.Errorf("%s: the patch mutated the stored profile", c.lang)
		}
	}
}

// A profile with no AppTitle element at all gets one, in the default language.
func TestNavPatchWebProfile_AppTitleOverNoStoredText(t *testing.T) {
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
	model.SetAuthoringLanguage("en_US")
	title := "EWO"
	for _, stored := range []any{nil, "absent"} {
		doc := appTitleProfile(nil, "")
		if stored == "absent" {
			doc = withoutKey(doc, "AppTitle")
		}
		out := mustNavPatchWebProfile(t, doc, types.NavigationProfileSpec{AppTitle: &title})
		if _, got := translationsOf(t, out); len(got) != 1 || got["en_US"] != "(fresh)=EWO" {
			t.Errorf("stored %v: items %v, want one en_US item 'EWO'", stored, got)
		}
	}
}

// An omitted clause keeps what is stored, as every other clause does
// (mendixlabs/mxcli#1522): the stored title is not reset to anything.
func TestNavPatchWebProfile_AppTitleAndIconKeptWhenSilent(t *testing.T) {
	stored := appTitleProfile(appTitleStored("en_US", "EWO", "nl_NL", "Mijn app"), "MyModule.Images.Logo")
	out := mustNavPatchWebProfile(t, stored, types.NavigationProfileSpec{LoginPage: "MyModule.Login"})
	if !bytes.Equal(mustMarshal(t, bson.D{{Key: "t", Value: navField(out, "AppTitle")}}), mustMarshal(t, bson.D{{Key: "t", Value: navField(stored, "AppTitle")}})) {
		t.Errorf("a silent statement changed AppTitle to %v", navField(out, "AppTitle"))
	}
	if got := navGetString(out, "AppIcon"); got != "MyModule.Images.Logo" {
		t.Errorf("a silent statement changed AppIcon to %q", got)
	}
}

func TestNavPatchWebProfile_AppIcon(t *testing.T) {
	stored := appTitleProfile(appTitleStored("en_US", "Mendix"), "Atlas_Core.Content.Mendix")
	for _, icon := range []string{"MyModule.Images.Logo", ""} {
		out := mustNavPatchWebProfile(t, stored, types.NavigationProfileSpec{AppIcon: &icon})
		if got, ok := navField(out, "AppIcon").(string); !ok || got != icon {
			t.Errorf("app icon %q written as %#v", icon, navField(out, "AppIcon"))
		}
	}
	if got := navGetString(stored, "AppIcon"); got != "Atlas_Core.Content.Mendix" {
		t.Errorf("the patch mutated the stored profile's AppIcon to %q", got)
	}
}

// The reader takes the title in the project's default language, by the rule a
// menu caption is read with, and the icon by name.
func TestAppTitleAndIconAreRead(t *testing.T) {
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
	doc := appTitleProfile(appTitleStored("en_US", "Mendix", "nl_NL", "Mijn app"), "Atlas_Core.Content.Mendix")
	for lang, want := range map[string]string{"en_US": "Mendix", "nl_NL": "Mijn app", "de_DE": "Mendix"} {
		model.SetAuthoringLanguage(lang)
		p := navProfileFromDoc(t, doc)
		if p.AppTitle != want {
			t.Errorf("%s: AppTitle read as %q, want %q", lang, p.AppTitle, want)
		}
		if p.AppIcon != "Atlas_Core.Content.Mendix" {
			t.Errorf("%s: AppIcon read as %q", lang, p.AppIcon)
		}
	}
	model.SetAuthoringLanguage("en_US")
	if p := navProfileFromDoc(t, appTitleProfile(appTitleStored(), "")); p.AppTitle != "" || p.AppIcon != "" {
		t.Errorf("an empty title and no icon read as %q, %q", p.AppTitle, p.AppIcon)
	}
}

// End to end on a real project: a second language's title survives a write of
// the default language's, a statement that does not mention the title keeps
// it, and restating what is stored is not a write.
func TestUpdateNavigationProfile_AppTitleKeepsOtherLanguages(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
	model.SetAuthoringLanguage("en_US")

	nav, err := b.GetNavigation()
	if err != nil || nav == nil {
		t.Fatalf("GetNavigation: %v", err)
	}
	prof := storeProfileField(t, b, "AppTitle", testCaption("en_US", "Mendix", "nl_NL", "Mijn app"))

	// Not the fixture's stored icon (Atlas_Core.Content.Mendix), so the write
	// is observable. The backend stores the name; resolving it is exec's job.
	title, icon := "EWO", "MyFirstModule.Images.Logo"
	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{AppTitle: &title, AppIcon: &icon}); err != nil {
		t.Fatal(err)
	}
	assertStoredTitle := func(when string) {
		t.Helper()
		got := map[string]string{}
		for _, it := range navGetArray(navGetDoc(rawProfile(t, b, prof), "AppTitle"), "Items") {
			if d, ok := it.(bson.D); ok {
				got[navGetString(d, "LanguageCode")] = navGetString(d, "Text")
			}
		}
		if len(got) != 2 || got["en_US"] != "EWO" || got["nl_NL"] != "Mijn app" {
			t.Errorf("%s: stored AppTitle %v, want en_US 'EWO' and nl_NL 'Mijn app'", when, got)
		}
		if g := navGetString(rawProfile(t, b, prof), "AppIcon"); g != icon {
			t.Errorf("%s: stored AppIcon %q, want %q", when, g, icon)
		}
	}
	assertStoredTitle("after app title 'EWO'")
	p := reloadProfile(t, proj, prof)
	if p.AppTitle != "EWO" || p.AppIcon != icon {
		t.Errorf("read back as title %q, icon %q", p.AppTitle, p.AppIcon)
	}

	// Silent: kept.
	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{}); err != nil {
		t.Fatal(err)
	}
	assertStoredTitle("after a statement without the clauses")

	// Restated: not a write. CONTROL: a different title is one.
	s0 := b.WriteStats()
	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{AppTitle: &title, AppIcon: &icon}); err != nil {
		t.Fatal(err)
	}
	s1 := b.WriteStats()
	if s1.Written != s0.Written {
		t.Errorf("restating the stored title and icon wrote the unit (%d -> %d writes)", s0.Written, s1.Written)
	}
	other := "EWO 2"
	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{AppTitle: &other}); err != nil {
		t.Fatal(err)
	}
	if s2 := b.WriteStats(); s2.Written != s1.Written+1 {
		t.Errorf("control: a changed title did not write (%d -> %d writes)", s1.Written, s2.Written)
	}
}

// rawProfile is the stored profile document of that name.
func rawProfile(t *testing.T, b *Backend, name string) bson.D {
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
		t.Fatal(err)
	}
	for _, p := range navGetArray(doc, "Profiles") {
		if d, ok := p.(bson.D); ok && navGetString(d, "Name") == name {
			return d
		}
	}
	t.Fatalf("no profile %q", name)
	return nil
}

// storeProfileField sets one field of the first web profile, written as a
// patch so nothing is re-paired or carried, and returns the profile name.
func storeProfileField(t *testing.T, b *Backend, key string, value any) string {
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
		t.Fatal(err)
	}
	profiles := navGetArray(doc, "Profiles")
	name := ""
	for i, p := range profiles {
		prof, ok := p.(bson.D)
		if !ok || navGetString(prof, "$Type") != "Navigation$NavigationProfile" {
			continue
		}
		profiles[i] = navSetField(prof, key, value)
		name = navGetString(prof, "Name")
		break
	}
	if name == "" {
		t.Fatal("fixture has no web profile")
	}
	if err := b.writer.UpdateRawUnitPatch(string(nav.ID), mustMarshal(t, navSetField(doc, "Profiles", profiles))); err != nil {
		t.Fatalf("write: %v", err)
	}
	return name
}
