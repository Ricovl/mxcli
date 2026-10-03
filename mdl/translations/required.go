// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// requiredCaptions are the texts mxbuild refuses to build when the project's
// default language has no text for them: CE4899 "Empty caption. [German,
// Germany]" (ako/mxcli#944), keyed "<OwnerType>.<Property>".
//
// Measured, not assumed — only flag what mxbuild flags. On a fresh 11.14.0 app
// a page title, a tab page caption, a group box caption, an action and a link
// button caption, a data grid column header, an input label, a dynamic text, a
// title widget, an enumeration value caption, a navigation menu item caption
// and a show-message text were each written with an en_US text only; the
// default language was then switched to de_DE (with en_US still enabled, and
// again with it dropped). `mx check` failed with CE4899 on the tab pages and on
// nothing else, both times — including the stock Administration.Account_Overview
// tabPage2. A caption kind belongs here only once a build has been seen to
// refuse it.
var requiredCaptions = map[string]string{
	"Forms$TabPage.Caption": "tab page caption",
}

// RequiredCaptionKind names a site mxbuild requires in the default language,
// or "" for a text it does not require.
func RequiredCaptionKind(s Site) string {
	return requiredCaptions[s.OwnerType+"."+s.Property]
}

// MissingCaption is one required caption with no text in the language asked
// about.
type MissingCaption struct {
	UnitID      model.ID
	ContainerID model.ID
	UnitType    string
	// UnitName is the document's own Name; the caller qualifies it.
	UnitName string
	// Kind is the caption kind ("tab page caption").
	Kind      string
	OwnerName string
	ElementID string
	// Sample is the text in another language, for the message; "" when the
	// caption is empty in every language.
	Sample string
}

// String is "tab page caption tabPage2 (\"Local Users\")".
func (m MissingCaption) String() string {
	s := m.Kind
	if m.OwnerName != "" {
		s += " " + m.OwnerName
	}
	if m.Sample != "" {
		s += fmt.Sprintf(" (%q)", m.Sample)
	}
	return s
}

// UnitReader is what MissingRequiredCaptions reads: the units and their bytes.
type UnitReader interface {
	ListUnits() ([]*types.UnitInfo, error)
	GetRawUnitBytes(id model.ID) ([]byte, error)
}

// MissingRequiredCaptions lists every required caption with no non-empty text
// in lang — the CE4899 set a build in that default language will report.
// Excluded documents are skipped: mxbuild does not check them.
func MissingRequiredCaptions(p UnitReader, lang string) ([]MissingCaption, error) {
	units, err := p.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("list units: %w", err)
	}
	var out []MissingCaption
	for _, u := range units {
		if u == nil {
			continue
		}
		raw, err := p.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		for _, m := range MissingRequiredCaptionsInUnit(u.ID, u.Type, raw, lang) {
			m.ContainerID = u.ContainerID
			out = append(out, m)
		}
	}
	return out, nil
}

// checkedUnitTypes are the documents mxbuild checks for CE4899. A page template
// and a building block hold tab pages too — a stock 11.14 app has 22 of them
// with en_US-only captions in Atlas_Web_Content — but they are blueprints Studio
// Pro copies from, and the de_DE build that failed on the one page reported
// none of them.
var checkedUnitTypes = map[string]bool{
	"Forms$Page":    true,
	"Forms$Snippet": true,
	"Forms$Layout":  true,
}

// MissingRequiredCaptionsInUnit is MissingRequiredCaptions over one unit.
func MissingRequiredCaptionsInUnit(id model.ID, unitType string, raw []byte, lang string) []MissingCaption {
	if !checkedUnitTypes[unitType] {
		return nil
	}
	var doc bson.D

	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	if excluded, _ := lookup(doc, "Excluded").(bool); excluded {
		return nil
	}
	name, _ := lookup(doc, "Name").(string)
	var out []MissingCaption
	for _, s := range SitesIn(doc) {
		kind := RequiredCaptionKind(s)
		if kind == "" || s.Targets[lang] != "" {
			continue
		}
		out = append(out, MissingCaption{
			UnitID: id, UnitType: unitType, UnitName: name, Kind: kind,
			OwnerName: s.OwnerName, ElementID: s.ElementID, Sample: sampleText(s.Targets),
		})
	}
	return out
}

// sampleText is a non-empty text from the lowest-sorting language, so a
// message names the caption the reader recognises.
func sampleText(targets map[string]string) string {
	for _, l := range sortedLanguages(targets) {
		if targets[l] != "" {
			return targets[l]
		}
	}
	return ""
}

func sortedLanguages(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
