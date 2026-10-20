// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func TestSelectTranslation(t *testing.T) {
	tr := func(pairs ...string) []TranslationEntry {
		var out []TranslationEntry
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, TranslationEntry{LanguageCode: pairs[i], Text: pairs[i+1]})
		}
		return out
	}
	cases := []struct {
		name    string
		entries []TranslationEntry
		lang    string
		want    string
	}{
		{"preferred wins over earlier entries", tr("en_US", "Home", "nl_NL", "Start"), "nl_NL", "Start"},
		{"preferred present but empty is empty", tr("nl_NL", "", "en_US", "Home"), "nl_NL", ""},
		{"en_US is the fallback", tr("de_DE", "Haus", "en_US", "Home"), "nl_NL", "Home"},
		{"first non-empty is the last resort", tr("de_DE", "", "fr_FR", "Maison"), "nl_NL", "Maison"},
		{"first entry of a repeated language", tr("nl_NL", "A", "nl_NL", "B"), "nl_NL", "A"},
		{"nothing stored", nil, "en_US", ""},
	}
	for _, c := range cases {
		if got := SelectTranslation(c.entries, c.lang); got != c.want {
			t.Errorf("%s: SelectTranslation = %q, want %q", c.name, got, c.want)
		}
	}
}
