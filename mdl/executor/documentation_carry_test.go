// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// A doc comment cannot spell trailing whitespace, a CRLF or a blank line at its
// edges: the visitor trims each line and the framing blank lines. (An interior
// blank line it does keep — mendixlabs/mxcli#1300.) So describe output of
// Studio Pro prose ("line\r\n\r\nline \r\n") re-executed as `create or modify`
// states the same text in that normal form, and overwriting the stored value
// with it rewrote the document with nothing changed (#731: 7 JavaScript actions
// in PedApp). When the stated text is exactly the stored text's normal form, the
// stored bytes are kept; any real edit still wins.
func TestCarriedDocumentation_KeepsStoredWhenOnlyTheSpellingDiffers(t *testing.T) {
	stored := "What does this JavaScript action do?\r\n\r\nWhen you upload a screenshot manually. \r\n\r\nReturn Type:\r\nWill return base 64 image string\r\n"
	cases := []struct {
		name   string
		set    bool
		stated string
		want   string
	}{
		{"no comment keeps stored", false, "", stored},
		{"normal form keeps stored bytes", true,
			"What does this JavaScript action do?\n\nWhen you upload a screenshot manually.\n\nReturn Type:\nWill return base 64 image string",
			stored},
		// The paragraph breaks are text: dropping them is an edit (#1300).
		{"collapsed paragraphs are an edit", true,
			"What does this JavaScript action do?\nWhen you upload a screenshot manually.\nReturn Type:\nWill return base 64 image string",
			"What does this JavaScript action do?\nWhen you upload a screenshot manually.\nReturn Type:\nWill return base 64 image string"},
		// Controls: a real change is written, and an empty comment still clears.
		{"edited text wins", true, "What does this JavaScript action do?\nSomething else.", "What does this JavaScript action do?\nSomething else."},
		{"empty comment clears", true, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := carriedDocumentation(c.set, c.stated, stored); got != c.want {
				t.Errorf("carriedDocumentation = %q, want %q", got, c.want)
			}
		})
	}
}
