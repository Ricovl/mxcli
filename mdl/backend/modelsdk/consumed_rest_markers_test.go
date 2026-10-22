// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// The typed-array markers of a consumed REST service are the ones Studio Pro
// writes: Operations, an operation's Headers and Parameters 3, QueryParameters
// 2, Tags 1. Measured on the Studio Pro-authored services in TestApp (11.14.0)
// and on `mx convert -p` of mxcli-written ones (11.6.6), which agree. The
// writer had 2 for the first three — copied from the legacy serializer, never
// measured — so re-executing describe output rewrote a Studio Pro-authored
// service it had not changed (`/Operations[0]: 3 -> 2`, mendixlabs/mxcli#1608
// once its describe output parsed).
func TestConsumedRestService_ListMarkersMatchStudioPro(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	svc := &model.ConsumedRestService{
		ContainerID: mod.ID,
		Name:        "MarkerProbe",
		BaseUrl:     "https://api.example.com",
		Operations: []*model.RestClientOperation{
			{Name: "WithParams", HttpMethod: "GET", Path: "/items/{id}", ResponseType: "JSON",
				Parameters:      []*model.RestClientParameter{{Name: "id", DataType: "String"}},
				QueryParameters: []*model.RestClientParameter{{Name: "q", DataType: "String"}}},
			{Name: "Bare", HttpMethod: "GET", Path: "/ping", ResponseType: "NONE"},
		},
	}
	if err := b.CreateConsumedRestService(svc); err != nil {
		t.Fatalf("CreateConsumedRestService: %v", err)
	}
	raw, err := b.GetRawUnitBytes(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if got := markerOf(t, doc, "Operations"); got != 3 {
		t.Errorf("Operations marker = %d, want 3", got)
	}
	var ops bson.A
	for _, e := range doc {
		if e.Key == "Operations" {
			ops = e.Value.(bson.A)
		}
	}
	for i, el := range ops[1:] {
		op := el.(bson.D)
		// Both the filled and the empty lists: an empty one is still written.
		for key, want := range map[string]int32{"Headers": 3, "Parameters": 3, "QueryParameters": 2, "Tags": 1} {
			if got := markerOf(t, op, key); got != want {
				t.Errorf("Operations[%d].%s marker = %d, want %d", i, key, got, want)
			}
		}
	}
}
