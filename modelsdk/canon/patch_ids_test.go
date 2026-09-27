// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// seqFlows builds a flow document whose Flows list holds one sequence flow per
// id in ids, each from origin i to origin i+1. Every flow has the same $Type
// and shape, which is exactly what makes the structural pairing positional.
func seqFlows(t *testing.T, ids ...byte) []byte {
	t.Helper()
	flows := bson.A{int32(3)}
	for i, id := range ids {
		flows = append(flows, bson.D{
			{Key: "$Type", Value: "Microflows$SequenceFlow"},
			{Key: "$ID", Value: bin(id)},
			{Key: "OriginPointer", Value: bin(byte(100 + i))},
			{Key: "DestinationPointer", Value: bin(byte(101 + i))},
		})
	}
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Flows", Value: flows},
	})
}

// A patch of the stored document — an `alter microflow` drop — keeps every
// surviving element's $ID already. The structural transplant must not then
// re-pair those elements: with one flow gone, the flows after it pair
// positionally with their predecessors and each takes its neighbour's $ID,
// moving identities onto other nodes (ADR-0012's "51 of 161 $IDs changed").
func TestReconcile_PatchOwnsElementIDs(t *testing.T) {
	stored := seqFlows(t, 10, 20, 30, 40)
	patched := seqFlows(t, 10, 30, 40) // flow 20 dropped; 30 and 40 keep their $IDs

	// Control: the rebuild path pairs by position and moves $IDs, which is
	// the defect the option exists for.
	rebuilt, _ := Reconcile(patched, stored)
	if ids := idSet(t, rebuilt); ids[blobToUUID(bin(40).Data)] {
		t.Fatalf("control: expected the transplant to move $ID 40 onto another flow; the test would prove nothing")
	}

	out, unchanged := Reconcile(patched, stored, ContentsOwnElementIDs())
	if unchanged {
		t.Fatal("a patch that dropped a flow reported no change")
	}
	if !bytes.Equal(out, patched) {
		t.Errorf("a patch that owns its element $IDs was rewritten by Reconcile")
	}
}
