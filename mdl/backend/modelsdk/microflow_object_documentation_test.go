// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// A split's and a loop's Documentation reach the read model, so the catalog's
// activities.Description carries them (mendixlabs/mxcli#1267). Before, only an
// action activity's documentation was read; the split writer already stored
// o.Documentation, so a read-modify-write of a flow also cleared every split's.
func TestFlowObjectFromGen_SplitAndLoopDocumentation(t *testing.T) {
	decode := func(d bson.D) microflows.MicroflowObject {
		t.Helper()
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(mustMarshalFlow(d))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return flowObjectFromGen(el)
	}

	split := decode(bson.D{
		{Key: "$ID", Value: "split-1"},
		{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: "Big?"},
		{Key: "Documentation", Value: "Routes large orders"},
	})
	if s, ok := split.(*microflows.ExclusiveSplit); !ok || s.Documentation != "Routes large orders" {
		t.Errorf("split = %#v, want Documentation read", split)
	}

	loop := decode(bson.D{
		{Key: "$ID", Value: "loop-1"},
		{Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "Documentation", Value: "One pass per order"},
	})
	if l, ok := loop.(*microflows.LoopedActivity); !ok || l.Documentation != "One pass per order" {
		t.Errorf("loop = %#v, want Documentation read", loop)
	}
}
