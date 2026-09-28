// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Studio Pro stores all four system-attribute flags of an entity's
// NoGeneralization, false ones included (ako/TestApp, Mendix 11.14.0:
// Clients.Orders and Odata.Devices; PedApp, 11.13.0). The writer set only the
// true ones, so every rewrite of a Studio Pro domain model deleted the false
// flags (#721 B), and a Studio Pro external entity could not keep its own
// describe output unchanged (#743).
func TestEntityToGen_WritesEveryGeneralizationFlag(t *testing.T) {
	for _, c := range []struct {
		e    domainmodel.Entity
		want map[string]bool
	}{
		{domainmodel.Entity{Name: "Orders"}, map[string]bool{
			"HasChangedByAttr": false, "HasChangedDateAttr": false, "HasCreatedDateAttr": false, "HasOwnerAttr": false}},
		{domainmodel.Entity{Name: "Audited", Persistable: true, HasOwner: true, HasChangedDate: true}, map[string]bool{
			"HasChangedByAttr": false, "HasChangedDateAttr": true, "HasCreatedDateAttr": false, "HasOwnerAttr": true}},
	} {
		raw, err := (&codec.Encoder{}).Encode(entityToGen(&c.e, "M", 11))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		g := bson.Raw(raw).Lookup("MaybeGeneralization").Document()
		for key, want := range c.want {
			v, err := g.LookupErr(key)
			if err != nil {
				t.Errorf("%s: %s not written (Studio Pro stores it)", c.e.Name, key)
				continue
			}
			if got, _ := v.BooleanOK(); got != want {
				t.Errorf("%s: %s = %v, want %v", c.e.Name, key, got, want)
			}
		}
	}
}
