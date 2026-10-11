// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-ledger FINDINGS #170, the writer half: SubmitBehaviour,
// SubmitOnInputDelay and Forms$LayoutGrid.Width were hard-coded, so a stored
// WhileEditing / FixedWidth was rewritten to the default on every exec.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func intRef(n int) *int { return &n }

func TestWriteTextInput_SubmitBehaviour(t *testing.T) {
	for _, tc := range []struct {
		name      string
		behaviour string
		delay     *int
		wantB     string
		wantD     int64
	}{
		// Unset writes Studio Pro's defaults, byte-for-byte as before.
		{"unset", "", nil, "OnEndEditing", 300},
		{"WhileEditing", "WhileEditing", nil, "WhileEditing", 300},
		{"WhileEditing with delay", "WhileEditing", intRef(750), "WhileEditing", 750},
		{"delay zero", "", intRef(0), "OnEndEditing", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := pages.BaseWidget{BaseElement: model.BaseElement{ID: "w"}, Name: "w"}
			for kind, w := range map[string]pages.Widget{
				"textbox":  &pages.TextBox{BaseWidget: base, SubmitBehaviour: tc.behaviour, SubmitOnInputDelay: tc.delay},
				"textarea": &pages.TextArea{BaseWidget: base, SubmitBehaviour: tc.behaviour, SubmitOnInputDelay: tc.delay},
			} {
				d := encodeWidget(t, w)
				if got := bsonnav.DGetString(d, "SubmitBehaviour"); got != tc.wantB {
					t.Errorf("%s: SubmitBehaviour = %q, want %q", kind, got, tc.wantB)
				}
				if got, ok := asInt64(bsonnav.DGet(d, "SubmitOnInputDelay")); !ok || got != tc.wantD {
					t.Errorf("%s: SubmitOnInputDelay = %v, want %d", kind, bsonnav.DGet(d, "SubmitOnInputDelay"), tc.wantD)
				}
			}
		})
	}
}

func TestWriteLayoutGrid_Width(t *testing.T) {
	for _, tc := range []struct{ authored, want string }{
		{"", "FullWidth"},
		{"FullWidth", "FullWidth"},
		{"FixedWidth", "FixedWidth"},
	} {
		d := encodeWidget(t, &pages.LayoutGrid{
			BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "lg"}, Name: "lg"},
			Width:      tc.authored,
		})
		if got := bsonnav.DGetString(d, "Width"); got != tc.want {
			t.Errorf("authored %q: Width = %q, want %q", tc.authored, got, tc.want)
		}
	}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}
