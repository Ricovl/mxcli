// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1676, the writer half: "a data view's `Editability: Never` becomes
// `Always`, so a read-only data view's inputs become editable", and "adding
// `Editable: Never` by hand is also ignored: `exec` reports 'Unchanged page' and
// the value stays `Always`".
//
// The page builder puts a data view's `Editable:` on the BaseWidget, as it does
// for every widget. The writer took the data view's Editability from
// DataView.ReadOnly instead, a field nothing sets, so it wrote Always whatever
// was authored.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestWriteDataView_Editability(t *testing.T) {
	for _, tc := range []struct {
		name string
		bw   pages.BaseWidget
		want string
	}{
		// Control: an unauthored data view keeps Mendix's default.
		{"unset", pages.BaseWidget{}, "Always"},
		{"never", pages.BaseWidget{Editable: "Never"}, "Never"},
		{"always", pages.BaseWidget{Editable: "Always"}, "Always"},
		// The conditional settings element is what makes it Conditional; writing
		// Always beside it contradicts the element.
		{"conditional", pages.BaseWidget{ConditionalEditability: &pages.ConditionalEditabilitySettings{
			BaseElement: model.BaseElement{TypeName: "Forms$ConditionalEditabilitySettings"},
			Expression:  "$currentObject/Locked = false",
		}}, "Conditional"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bw := tc.bw
			bw.BaseElement = model.BaseElement{ID: "dv"}
			bw.Name = "dataView1"
			d := encodeWidget(t, &pages.DataView{BaseWidget: bw})
			if got := bsonnav.DGetString(d, "Editability"); got != tc.want {
				t.Errorf("Editability = %q, want %q", got, tc.want)
			}
		})
	}
}
