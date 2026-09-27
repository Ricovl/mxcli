// SPDX-License-Identifier: Apache-2.0

package backend

import "testing"

// The explicit column address travels through the PageMutator interface as its
// columnRef string, so the spelling String writes must be the one
// ParseColumnSelector reads back — including a caption holding the characters
// the spelling itself uses.
func TestColumnSelectorRoundTrips(t *testing.T) {
	for _, s := range []ColumnSelector{
		{Attribute: "Name"},
		{Attribute: "UserRoles/Name"},
		{Attribute: "FullName", Ordinal: 2},
		{Caption: "Total"},
		{Caption: "it's (net)"},
		{Caption: " ", Ordinal: 1},
	} {
		ref := s.String()
		got, ok := ParseColumnSelector(ref)
		if !ok {
			t.Errorf("ParseColumnSelector(%q) = not a selector", ref)
			continue
		}
		if got != s {
			t.Errorf("ParseColumnSelector(%q) = %+v, want %+v", ref, got, s)
		}
	}
}

// A derived column name — what `grid.Name` has always passed — is never read as
// a selector: none contains "(" (they are an attribute, a sanitized caption or
// colN).
func TestParseColumnSelectorRejectsDerivedNames(t *testing.T) {
	for _, ref := range []string{"", "Name", "col3", "UserRoles/Name", "column", "column(", "column()", "column('x'", "column(Name)@", "column(Name)@0", "column(Name)x"} {
		if s, ok := ParseColumnSelector(ref); ok {
			t.Errorf("ParseColumnSelector(%q) = %+v, want not a selector", ref, s)
		}
	}
}

func TestColumnSelectorInMessages(t *testing.T) {
	sel := ColumnSelector{Attribute: "Name", Ordinal: 2}.String()
	if got := (AlterTarget{Path: []string{"dg", sel}}).String(); got != "dg column(Name)@2" {
		t.Errorf("AlterTarget.String() = %q", got)
	}
	if got := (WidgetRef{Widget: "dg", Column: sel}).Name(); got != "dg column(Name)@2" {
		t.Errorf("WidgetRef.Name() = %q", got)
	}
	if got := (WidgetRef{Widget: "dg", Column: "Name"}).Name(); got != "dg.Name" {
		t.Errorf("legacy WidgetRef.Name() = %q", got)
	}
}
