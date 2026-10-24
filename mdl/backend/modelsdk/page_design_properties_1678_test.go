// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1678, the writer half: "exec writes an empty list" for the page's
// own Appearance.DesignProperties — pageToGen passed nil to newAppearance.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestPageToGen_WritesPageDesignProperties(t *testing.T) {
	page := &pages.Page{Name: "Home_Native", DesignProperties: []pages.DesignPropertyValue{
		{Key: "Background color", ValueType: "option", Option: "Background Primary"},
		{Key: "Hide header bottom border", ValueType: "toggle"},
	}}
	g, err := pageToGen(page, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(d, "Appearance"), "DesignProperties")) {
		if dp, ok := el.(bson.D); ok {
			keys = append(keys, bsonnav.DGetString(dp, "Key"))
		}
	}
	if len(keys) != 2 || keys[0] != "Background color" || keys[1] != "Hide header bottom border" {
		t.Errorf("page Appearance.DesignProperties keys = %v, want the two authored", keys)
	}
}
