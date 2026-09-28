// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// TestUpdateConsumedODataService_CarriesStoredDocument pins the wiring of
// carryStoredConsumedODataService into UpdateConsumedODataService (#743). The
// overlay's own tests call it directly, so without this one the call could be
// dropped from the write path and only the integration round trip would see it.
func TestUpdateConsumedODataService_CarriesStoredDocument(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	svc := &model.ConsumedODataService{
		ContainerID:       mod.ID,
		Name:              "ZzClient",
		ServiceName:       "ZzClient",
		ODataVersion:      "OData4",
		TimeoutExpression: "300",
		ProxyType:         "DefaultProxy",
	}
	if err := b.CreateConsumedODataService(svc); err != nil {
		t.Fatalf("CreateConsumedODataService: %v", err)
	}

	// Stand in for a Studio Pro client: UseQuerySegment true and an Icon, neither
	// of which the model holds.
	raw, err := b.reader.GetRawUnitBytes(string(svc.ID))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bsonnav.DSet(d, "UseQuerySegment", true)
	d = append(d, bson.E{Key: "Icon", Value: []byte{0x89, 'P', 'N', 'G'}})
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(svc.ID), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}

	svc.TimeoutExpression = "60"
	if err := b.UpdateConsumedODataService(svc); err != nil {
		t.Fatalf("UpdateConsumedODataService: %v", err)
	}

	raw, err = b.reader.GetRawUnitBytes(string(svc.ID))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var after bson.D
	if err := bson.Unmarshal(raw, &after); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := bsonnav.DGet(after, "UseQuerySegment").(bool); !v {
		t.Errorf("UseQuerySegment = %v, want the stored true", bsonnav.DGet(after, "UseQuerySegment"))
	}
	if bsonnav.DGet(after, "Icon") == nil {
		t.Errorf("Icon dropped by the rewrite")
	}
	// Control: the authored change still lands.
	if got := bsonnav.DGetString(after, "TimeoutExpression"); got != "60" {
		t.Errorf("TimeoutExpression = %q, want the declared 60", got)
	}
}
