// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#721 L3. Inside a snippet, `dataview (DataSource: $Task/Assoc)` is
// traversed from a SNIPPET parameter, and Studio Pro names it in the
// SnippetParameter slot of the source's Forms$PageVariable (TestApp's
// WorkflowCommons.Snip_UserTask_TaskTimeline). Both association writers put
// every context variable in the PageParameter slot — a page parameter the
// snippet does not have, which resolves to nothing.
func TestAssociationSourceContextVariableSlot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		widget   func(ds pages.DataSource) pages.Widget
		inSnip   bool
		wantSlot string
	}{
		{"data view, page parameter", dataViewWith, false, "PageParameter"},
		{"data view, snippet parameter", dataViewWith, true, "SnippetParameter"},
		{"list view, page parameter", listViewWith, false, "PageParameter"},
		{"list view, snippet parameter", listViewWith, true, "SnippetParameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := encodeWidget(t, tc.widget(&pages.AssociationSource{
				BaseElement:        model.BaseElement{TypeName: "Forms$AssociationSource"},
				EntityPath:         "Sales.Order_Customer/Sales.Customer",
				ContextVariable:    "Order",
				IsSnippetParameter: tc.inSnip,
			}))
			src, ok := docGet(doc, "DataSource").(bsonv1.D)
			if !ok {
				t.Fatalf("DataSource not serialized (got %T)", docGet(doc, "DataSource"))
			}
			sv, ok := docGet(src, "SourceVariable").(bsonv1.D)
			if !ok {
				t.Fatalf("SourceVariable not a document (got %T)", docGet(src, "SourceVariable"))
			}
			for _, slot := range []string{"PageParameter", "SnippetParameter"} {
				want := ""
				if slot == tc.wantSlot {
					want = "Order"
				}
				if got := docGet(sv, slot); got != want {
					t.Errorf("SourceVariable.%s = %v, want %q", slot, got, want)
				}
			}
		})
	}
}

func dataViewWith(ds pages.DataSource) pages.Widget {
	dv := &pages.DataView{DataSource: ds}
	dv.Name = "dv"
	return dv
}

func listViewWith(ds pages.DataSource) pages.Widget {
	lv := &pages.ListView{DataSource: ds}
	lv.Name = "lv"
	return lv
}
