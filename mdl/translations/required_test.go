// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// probePage is the measured case (ako/mxcli#944): a page whose texts are all
// en_US only. With de_DE the default, mxbuild reports CE4899 on the tab page
// caption and on nothing else, so the tab page is the only one to report.
func probePage(t *testing.T, excluded bool, tabCaption bson.D) []byte {
	t.Helper()
	doc := bson.D{
		{Key: "$ID", Value: "page-id"},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Account_Overview"},
		{Key: "Excluded", Value: excluded},
		{Key: "Title", Value: text(tr("en_US", "Accounts"))},
		{Key: "Widgets", Value: bson.A{
			bson.D{
				{Key: "$ID", Value: "tab-id"},
				{Key: "$Type", Value: "Forms$TabPage"},
				{Key: "Name", Value: "tabPage2"},
				{Key: "Caption", Value: tabCaption},
			},
			bson.D{
				{Key: "$ID", Value: "btn-id"},
				{Key: "$Type", Value: "Forms$ActionButton"},
				{Key: "Name", Value: "btn1"},
				{Key: "Caption", Value: text(tr("en_US", "Save"))},
			},
		}},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMissingRequiredCaptions_OnlyTheTabPageCaption(t *testing.T) {
	raw := probePage(t, false, text(tr("en_US", "Local Users"), tr("nl_NL", "Lokale gebruikers")))

	got := MissingRequiredCaptionsInUnit("u1", "Forms$Page", raw, "de_DE")
	if len(got) != 1 {
		t.Fatalf("want exactly the tab page (page title and button caption are not required), got %+v", got)
	}
	m := got[0]
	if m.Kind != "tab page caption" || m.OwnerName != "tabPage2" || m.UnitName != "Account_Overview" ||
		m.Sample != "Local Users" {
		t.Errorf("got %+v", m)
	}
	if s := m.String(); s != `tab page caption tabPage2 ("Local Users")` {
		t.Errorf("String() = %s", s)
	}

	// Control: the same page in its own default language reports nothing.
	if got := MissingRequiredCaptionsInUnit("u1", "Forms$Page", raw, "en_US"); len(got) != 0 {
		t.Errorf("en_US is present; want nothing, got %+v", got)
	}
}

func TestMissingRequiredCaptions_EmptyTextCountsAsMissing(t *testing.T) {
	raw := probePage(t, false, text(tr("de_DE", ""), tr("en_US", "Local Users")))
	if got := MissingRequiredCaptionsInUnit("u1", "Forms$Page", raw, "de_DE"); len(got) != 1 {
		t.Errorf("an empty de_DE text is the CE4899 case; got %+v", got)
	}
	none := probePage(t, false, text())
	got := MissingRequiredCaptionsInUnit("u1", "Forms$Page", none, "de_DE")
	if len(got) != 1 || got[0].Sample != "" {
		t.Errorf("a caption with no text at all is missing too; got %+v", got)
	}
}

func TestMissingRequiredCaptions_SkipsExcludedDocuments(t *testing.T) {
	raw := probePage(t, true, text(tr("en_US", "Local Users")))
	if got := MissingRequiredCaptionsInUnit("u1", "Forms$Page", raw, "de_DE"); len(got) != 0 {
		t.Errorf("mxbuild does not check an excluded document; got %+v", got)
	}
}

// A page template and a building block are blueprints: the de_DE build that
// failed on Account_Overview reported none of the 22 en_US-only tab pages in
// Atlas_Web_Content's templates. Control: the same bytes as a page are reported.
func TestMissingRequiredCaptions_SkipsTemplates(t *testing.T) {
	raw := probePage(t, false, text(tr("en_US", "Tab 1")))
	for _, ty := range []string{"Forms$PageTemplate", "Forms$BuildingBlock"} {
		if got := MissingRequiredCaptionsInUnit("u1", ty, raw, "de_DE"); len(got) != 0 {
			t.Errorf("%s is not built; got %+v", ty, got)
		}
	}
	for _, ty := range []string{"Forms$Page", "Forms$Snippet"} {
		if got := MissingRequiredCaptionsInUnit("u1", ty, raw, "de_DE"); len(got) != 1 {
			t.Errorf("%s: want the tab page, got %+v", ty, got)
		}
	}
}
