// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Studio Pro writes an external entity's Rest$ODataKey Parts list with the
// typed-array marker 2 (measured on the Studio Pro-authored ako/TestApp,
// Clients.Orders); the encoder's default is 3, so every domain-model write
// rewrote the key of every external entity in the unit (#743).
func TestODataKeyToGen_PartsMarkerIsTwo(t *testing.T) {
	key := odataKeyToGen([]*domainmodel.RemoteKeyPart{{
		Name: "OrderId", RemoteName: "OrderId", RemoteType: "Edm.Int64",
		Type: &domainmodel.LongAttributeType{},
	}})
	b, err := (&codec.Encoder{}).Encode(key)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, e := range doc {
		if e.Key != "Parts" {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok || len(arr) != 2 {
			t.Fatalf("Parts = %v, want [marker, part]", e.Value)
		}
		if m, ok := arr[0].(int32); !ok || m != 2 {
			t.Errorf("Parts marker = %v, want 2", arr[0])
		}
		return
	}
	t.Fatalf("encoded key has no Parts: %v", doc)
}
