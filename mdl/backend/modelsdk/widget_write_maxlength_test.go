// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1235: a text box or text area bound to a page variable has
// no attribute length; -1 there is mx check CE6553.
func TestMaxLengthCodeFor(t *testing.T) {
	cases := []struct {
		name string
		sv   *pages.WidgetVariable
		want int32
	}{
		{"attribute binding", nil, -1},
		{"through a data view", &pages.WidgetVariable{Widget: "dv", Variable: "Account"}, -1},
		{"page variable", &pages.WidgetVariable{Variable: "Filter", Kind: "local"}, 0},
	}
	for _, c := range cases {
		if got := maxLengthCodeFor(c.sv); got != c.want {
			t.Errorf("%s: MaxLengthCode = %d, want %d", c.name, got, c.want)
		}
	}
}
