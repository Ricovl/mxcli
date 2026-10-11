// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1097. The consumed-REST inline mapping writer (Body: MAPPING and
// Response: MAPPING) still wrote IsDefaultType on every
// Import/ExportMappings$ValueMappingElement after #882 fixed the standalone
// import/export-mapping writer — see TestMappingValueElementsOmitIsDefaultType
// for the authority. Mendix declares isDefaultType on the OBJECT mapping element
// only, so `mx diff`/merge throws on the undeclared key.
func TestRestInlineMappingValueElementsOmitIsDefaultType(t *testing.T) {
	mappings := []*model.RestResponseMapping{
		{Attribute: "Name", ExposedName: "name"},
		{
			Entity: "M.Child", Association: "M.Child_Root", ExposedName: "child",
			Children: []*model.RestResponseMapping{
				{Attribute: "Code", ExposedName: "code"},
			},
		},
	}

	for _, tc := range []struct {
		name string
		g    element.Element
	}{
		{"response", restImplicitMappingResponseToGen("M.Root", mappings)},
		{"body", restImplicitMappingBodyToGen("M.Root", mappings)},
	} {
		raw, err := (&codec.Encoder{}).Encode(tc.g)
		if err != nil {
			t.Fatalf("%s: encode: %v", tc.name, err)
		}
		root, ok := bson.Raw(raw).Lookup("RootMappingElement").DocumentOK()
		if !ok {
			t.Fatalf("%s: no RootMappingElement", tc.name)
		}
		var objects, values int
		var walk func(doc bson.Raw)
		walk = func(doc bson.Raw) {
			_, present := doc.Lookup("IsDefaultType").BooleanOK()
			switch rawStr(doc, "ElementType") {
			case "Object":
				objects++
				if !present {
					t.Errorf("%s: OBJECT element %q lost IsDefaultType — it owns the property",
						tc.name, rawStr(doc, "JsonPath"))
				}
			case "Value":
				values++
				if present {
					t.Errorf("%s: VALUE element %q carries IsDefaultType, which "+
						"ValueMappingElement does not declare — mx diff/merge throws on it",
						tc.name, rawStr(doc, "JsonPath"))
				}
			}
			children, ok := doc.Lookup("Children").ArrayOK()
			if !ok {
				return
			}
			vals, err := children.Values()
			if err != nil {
				t.Fatalf("%s: read Children: %v", tc.name, err)
			}
			for _, v := range vals {
				if child, ok := v.DocumentOK(); ok {
					walk(child)
				}
			}
		}
		walk(root)
		if objects != 2 || values != 2 {
			t.Errorf("%s: walked %d object / %d value elements, want 2 / 2 — the assertion proved nothing",
				tc.name, objects, values)
		}
	}
}
