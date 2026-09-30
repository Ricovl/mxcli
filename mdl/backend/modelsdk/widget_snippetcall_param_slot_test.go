// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#721 L3, the snippet-call sibling. A snippet call that passes the
// enclosing snippet's own parameter names it in the SnippetParameter slot of the
// mapping's Forms$PageVariable (TestApp's WorkflowCommons snippets, 27 mappings).
// The writer hard-coded the PageParameter slot — a page parameter the snippet
// does not have, CE0115 at mx check.
func TestSnippetCallMappingVariableSlot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		inSnip   bool
		wantSlot string
	}{
		{"page parameter", false, "PageParameter"},
		{"snippet parameter", true, "SnippetParameter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := &pages.SnippetCallWidget{
				SnippetName: "Sales.OrderActions",
				ParameterMappings: []pages.SnippetParamMapping{
					{ParamName: "Order", Argument: "$Order", IsSnippetParameter: tc.inSnip},
				},
			}
			sc.Name = "sc1"
			doc := encodeWidget(t, sc)
			call, ok := docGet(doc, "FormCall").(bsonv1.D)
			if !ok {
				t.Fatalf("FormCall not serialized (got %T)", docGet(doc, "FormCall"))
			}
			mappings, ok := docGet(call, "ParameterMappings").(bsonv1.A)
			if !ok || len(mappings) != 2 {
				t.Fatalf("ParameterMappings = %#v, want [marker, mapping]", docGet(call, "ParameterMappings"))
			}
			m, _ := mappings[1].(bsonv1.D)
			pv, ok := docGet(m, "Variable").(bsonv1.D)
			if !ok {
				t.Fatalf("Variable not a document (got %T)", docGet(m, "Variable"))
			}
			for _, slot := range []string{"PageParameter", "SnippetParameter"} {
				want := ""
				if slot == tc.wantSlot {
					want = "Order"
				}
				if got := docGet(pv, slot); got != want {
					t.Errorf("Variable.%s = %v, want %q", slot, got, want)
				}
			}
		})
	}
}
