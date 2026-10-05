// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMsg "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
)

// The fixtures are the two MessageDefinition2 documents `mx convert` (11.15.0)
// wrote for the collection MsgTest.MD_Order made by
// mdl-examples/doctype-tests/40-message-definition-examples.mdl on 11.14 — the
// Studio Pro-authored reference for the 11.15 shape (ako/mxcli#987). `mx check`
// on the converted project: 0 errors.
//
// OrderMessage carries an association reached from its FROM side (MaxOccurs 1),
// CustomerOrders the same association from its TO side (-1).
var messageDocumentFixtures = []string{
	"testdata/MsgTest.OrderMessage.11_15.bson",
	"testdata/MsgTest.CustomerOrders.11_15.bson",
}

// TestMessageDefinitionDocumentRoundTrips reads each stored 11.15 document into
// the semantic model and writes it back, comparing against the STORED bytes.
func TestMessageDefinitionDocumentRoundTrips(t *testing.T) {
	for _, path := range messageDocumentFixtures {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			g := genMsg.NewMessageDefinition2()
			g.InitFromRaw(bson.Raw(raw))

			d := messageDocumentFromGen(g, "")
			if d.Name == "" || d.Root == nil {
				t.Fatalf("read nothing: name %q, root %v", d.Name, d.Root)
			}
			rebuilt, err := messageDocumentToGen(d)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			after, err := (&codec.Encoder{}).Encode(rebuilt)
			if err != nil {
				t.Fatalf("encode rebuilt: %v", err)
			}
			for _, diff := range bsonDiff(t, raw, after) {
				t.Errorf("DIFF %s", diff)
			}

			// Control: the comparison sees a change, so a green run above is
			// not two empty documents compared.
			d.Root.ExposedName += "X"
			changed, err := messageDocumentToGen(d)
			if err != nil {
				t.Fatalf("re-encode control: %v", err)
			}
			changedBytes, err := (&codec.Encoder{}).Encode(changed)
			if err != nil {
				t.Fatalf("encode control: %v", err)
			}
			if len(bsonDiff(t, raw, changedBytes)) == 0 {
				t.Error("control: a changed ExposedName produced no diff — the comparison sees nothing")
			}
		})
	}
}
