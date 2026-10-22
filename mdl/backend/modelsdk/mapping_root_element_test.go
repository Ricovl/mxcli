// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genExp "github.com/mendixlabs/mxcli/modelsdk/gen/exportmappings"
	genImp "github.com/mendixlabs/mxcli/modelsdk/gen/importmappings"
)

// An XML-schema mapping's root element is stored as XsdRootElementName
// (mendixlabs/mxcli#1623). Two things lost it: gen read the SDK name
// RootElementName, so the stored root read back as "", and the writers wrote
// XsdRootElementName: "" whatever the model held. Encoding a root and decoding
// it again must give it back, under the key Mendix reads.
func TestMappingRootElementName_RoundTrips(t *testing.T) {
	im := &model.ImportMapping{Name: "IMM", XmlSchema: "XM.XS_Order", RootElementName: "Order"}
	em := &model.ExportMapping{Name: "EXM", XmlSchema: "XM.XS_Order", RootElementName: "Order"}
	for kind, g := range map[string]element.Element{
		"import": importMappingToGen(im),
		"export": exportMappingToGen(em),
	} {
		raw, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatalf("%s: encode: %v", kind, err)
		}
		if got, _ := bson.Raw(raw).Lookup("XsdRootElementName").StringValueOK(); got != "Order" {
			t.Errorf("%s mapping: XsdRootElementName = %q, want %q", kind, got, "Order")
		}
		decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
		if err != nil {
			t.Fatalf("%s: decode: %v", kind, err)
		}
		var root string
		switch d := decoded.(type) {
		case *genImp.ImportMapping:
			root = d.RootElementName()
		case *genExp.ExportMapping:
			root = d.RootElementName()
		default:
			t.Fatalf("%s: decoded as %T", kind, decoded)
		}
		if root != "Order" {
			t.Errorf("%s mapping: the stored root element reads back as %q, want %q", kind, root, "Order")
		}
	}
}
