// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#977 item 2: "The text area's `NumberOfLines` and `Autocomplete`
// are not round-tripped." The writer turned an unset row count — and a stored
// 0 — into 5, and wrote no Autocomplete at all.
//
// Measured in PedApp and TestApp: NumberOfLines 5 on 52 of 58 text areas, 2 on
// 4 and 0 on 2; Autocomplete true on all 58.
func TestTextAreaNumberOfLinesAndAutocomplete_Written(t *testing.T) {
	n := func(v int) *int { return &v }
	b := func(v bool) *bool { return &v }
	for _, tc := range []struct {
		name         string
		lines        *int
		autocomplete *bool
		wantLines    int64
		wantAuto     bool
	}{
		{"unset keeps the defaults", nil, nil, 5, true},
		{"two lines", n(2), nil, 2, true},
		{"zero lines, as Studio Pro stores it", n(0), nil, 0, true},
		{"autocomplete off", nil, b(false), 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ta := &pages.TextArea{NumberOfLines: tc.lines, Autocomplete: tc.autocomplete}
			ta.Name = "textArea1"
			doc := encodeWidget(t, ta)
			var lines int64
			switch v := docGet(doc, "NumberOfLines").(type) {
			case int32:
				lines = int64(v)
			case int64:
				lines = v
			}
			if lines != tc.wantLines {
				t.Errorf("NumberOfLines = %v, want %d", docGet(doc, "NumberOfLines"), tc.wantLines)
			}
			if got := docGet(doc, "Autocomplete"); got != tc.wantAuto {
				t.Errorf("Autocomplete = %v, want %v", got, tc.wantAuto)
			}
		})
	}
}
