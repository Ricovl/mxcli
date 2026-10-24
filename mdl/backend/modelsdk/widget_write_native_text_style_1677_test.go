// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1677, the writer half: the writer stored NativeTextStyle Text on
// every dynamic text, so a native heading round-tripped into body text.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestWriteDynamicText_NativeTextStyle(t *testing.T) {
	for _, tc := range []struct {
		authored pages.NativeTextStyle
		want     string
	}{
		{"", "Text"}, // control: unauthored keeps the stored default
		{pages.NativeTextStyleHeading3, "Heading3"},
		{pages.NativeTextStyleHeading1, "Heading1"},
	} {
		d := encodeWidget(t, &pages.DynamicText{
			BaseWidget:      pages.BaseWidget{BaseElement: model.BaseElement{ID: "dt"}, Name: "text4"},
			NativeTextStyle: tc.authored,
		})
		if got := bsonnav.DGetString(d, "NativeTextStyle"); got != tc.want {
			t.Errorf("authored %q: NativeTextStyle = %q, want %q", tc.authored, got, tc.want)
		}
	}
}
