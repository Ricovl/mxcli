// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
)

// Mendix 11.15 removed a mapping's MessageDefinition key: a converted or new
// 11.15 mapping has only MessageDefinition2 (ako/mxcli#987). The writer used to
// add `MessageDefinition: ""` to every mapping, which is a property the 11.15
// document does not have. Its presence is now carried like MessageDefinition2:
// nil writes no key, a pointer writes it.
func TestMappingWriter_MessageDefinitionKeyPresence(t *testing.T) {
	strp := func(s string) *string { return &s }
	cases := []struct {
		name    string
		md, md2 *string
	}{
		{"11.15 shape: MessageDefinition absent", nil, strp("")},
		{"11.15 shape with a source", nil, strp("MsgTest.OrderMessage")},
		{"11.10-11.14 shape", strp(""), strp("")},
		{"pre-11.10 shape", strp("MsgTest.MD_Order.OrderMessage"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := &model.ImportMapping{Name: "IMM", MessageDefinition: tc.md, MessageDefinition2: tc.md2}
			em := &model.ExportMapping{Name: "EXM", MessageDefinition: tc.md, MessageDefinition2: tc.md2}
			for kind, g := range map[string]element.Element{
				"import": importMappingToGen(im),
				"export": exportMappingToGen(em),
			} {
				raw, err := (&codec.Encoder{}).Encode(g)
				if err != nil {
					t.Fatalf("%s: encode: %v", kind, err)
				}
				checkOptionalKey(t, kind, bson.Raw(raw), "MessageDefinition", tc.md)
				checkOptionalKey(t, kind, bson.Raw(raw), "MessageDefinition2", tc.md2)
			}
		})
	}
}

func checkOptionalKey(t *testing.T, kind string, raw bson.Raw, key string, want *string) {
	t.Helper()
	got := optionalStringFromRaw(raw, key)
	switch {
	case want == nil && got != nil:
		t.Errorf("%s mapping: %s = %q written, want the key absent", kind, key, *got)
	case want != nil && got == nil:
		t.Errorf("%s mapping: %s absent, want %q", kind, key, *want)
	case want != nil && *got != *want:
		t.Errorf("%s mapping: %s = %q, want %q", kind, key, *got, *want)
	}
}
