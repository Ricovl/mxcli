// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1652 (mendixlabs/mxcli#1643):
//
//	textfilter tfName (Attributes: [Name], Placeholder: 'Search name', Adjustable: false)
//
// passed check and exec inside `create page`, and neither value was stored —
// while `alter page … set (placeholder: …, adjustable: …)` on the same widget
// wrote both.
//
// Neither key is mapped by the filters' hand-written definitions, so both reach
// the explicit-property pass, which looked the author's name up in the
// template's property keys BYTE-FOR-BYTE. `Adjustable` is not `adjustable`; and
// `Placeholder:` is a grammar keyword the visitor always lowers to "Placeholder",
// so even the lower-case spelling DESCRIBE prints was dropped. check judged both
// keys case-insensitively and called them persisted, and the ALTER setter has
// matched case-insensitively since #1069 — the create path was the one reader
// that disagreed.

func filterWidgetDef(t *testing.T, keyword string) *WidgetDefinition {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get(keyword)
	if !ok {
		t.Fatalf("no %s definition", keyword)
	}
	return def
}

func buildFilterWidget(t *testing.T, keyword, src string) *pages.CustomWidget {
	t.Helper()
	w := parseSnippetWidget(t, src)
	cw, err := dropdownFilterEngine(t).Build(filterWidgetDef(t, keyword), w)
	if err != nil {
		t.Fatalf("Build %s: %v", src, err)
	}
	return cw
}

// storedTemplateText returns the text of a TextTemplate property's first
// translation, "" when the template is null or empty.
func storedTemplateText(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	tmpl, _ := bsonField(customWidgetValue(t, cw, key), "TextTemplate").(bson.D)
	text, _ := bsonField(tmpl, "Template").(bson.D)
	items, _ := bsonField(text, "Items").(bson.A)
	for _, it := range items {
		if tr, ok := it.(bson.D); ok {
			s, _ := bsonField(tr, "Text").(string)
			return s
		}
	}
	return ""
}

func TestBuild_FilterPlaceholderAndAdjustableAreStoredInAnyCase(t *testing.T) {
	cases := []struct {
		name, keyword, src string
		wantAdjustable     string
	}{
		// The report's spelling.
		{"textfilter, report spelling", "TEXTFILTER",
			"textfilter tf (Attributes: [Name], Placeholder: 'Search name', Adjustable: false)", "false"},
		// The spelling DESCRIBE prints, fed back into create.
		{"textfilter, describe spelling", "TEXTFILTER",
			"textfilter tf (Attributes: [Name], adjustable: false, placeholder: 'Search name')", "false"},
		// The two filters that declare the same pair.
		{"numberfilter", "NUMBERFILTER",
			"numberfilter nf (Placeholder: 'Search name', Adjustable: false)", "false"},
		{"datefilter", "DATEFILTER",
			"datefilter df (Placeholder: 'Search name', Adjustable: false)", "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cw := buildFilterWidget(t, tc.keyword, tc.src)
			if got := storedTemplateText(t, cw, "placeholder"); got != "Search name" {
				t.Errorf("placeholder stored %q, want %q (ako/mxcli#1652)", got, "Search name")
			}
			if got := storedPrimitive(t, cw, "adjustable"); got != tc.wantAdjustable {
				t.Errorf("adjustable stored %q, want %q (ako/mxcli#1652)", got, tc.wantAdjustable)
			}
		})
	}
}

// Any unmapped key, not only the reported two: the text and number filters'
// `delay` is an Integer the definitions do not map either.
func TestBuild_UnmappedFilterKeyIsStoredInAnyCase(t *testing.T) {
	cw := buildFilterWidget(t, "TEXTFILTER", "textfilter tf (Attributes: [Name], Delay: 250)")
	if got := storedPrimitive(t, cw, "delay"); got != "250" {
		t.Errorf("delay stored %q, want \"250\"", got)
	}
}

// CONTROL: left unset, the template's defaults stay — the fix must not write
// anything the script did not name.
func TestBuild_FilterDefaultsUnchangedWhenUnset(t *testing.T) {
	cw := buildFilterWidget(t, "TEXTFILTER", "textfilter tf (Attributes: [Name])")
	if got := storedTemplateText(t, cw, "placeholder"); got != "" {
		t.Errorf("placeholder stored %q with none authored, want empty", got)
	}
	if got := storedPrimitive(t, cw, "adjustable"); got != "true" {
		t.Errorf("adjustable stored %q with none authored, want the default \"true\"", got)
	}
}

// explicitPropertyKey folds case only where the answer is unambiguous and the
// pass can write the kind: an exact match always wins; a case-only match must be
// unique and of a kind check calls persisted.
func TestExplicitPropertyKey(t *testing.T) {
	ids := map[string]pages.PropertyTypeIDEntry{
		"placeholder": {ValueType: "TextTemplate"},
		"adjustable":  {ValueType: "Boolean"},
		"icon":        {ValueType: "Icon"},
		"mode":        {ValueType: "Enumeration"},
		"Mode":        {ValueType: "Enumeration"},
	}
	cases := []struct {
		name, want string
		ok         bool
	}{
		{"placeholder", "placeholder", true},
		{"Placeholder", "placeholder", true},
		{"ADJUSTABLE", "adjustable", true},
		// Exact wins even where a fold would be ambiguous.
		{"Mode", "Mode", true},
		{"mode", "mode", true},
		// Ambiguous by case alone: not guessed.
		{"MODE", "", false},
		// A kind the pass would store as a meaningless PrimitiveValue; check
		// warns it is dropped (MDL-WIDGET06), so it stays dropped.
		{"Icon", "", false},
		// CONTROL: the exact spelling of that kind behaves as before.
		{"icon", "icon", true},
		{"nosuch", "", false},
	}
	for _, tc := range cases {
		got, _, ok := explicitPropertyKey(ids, tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("explicitPropertyKey(%q) = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
