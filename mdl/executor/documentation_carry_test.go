// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// A doc comment cannot spell a blank line, trailing whitespace or a CRLF: the
// visitor keeps only the trimmed, non-empty lines. So describe output of
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
			"What does this JavaScript action do?\nWhen you upload a screenshot manually.\nReturn Type:\nWill return base 64 image string",
			stored},
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
