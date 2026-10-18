// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1232 (ako/mxcli#1290): re-executing the describe output of
// an `import from mapping` whose mapping is sourced from a web service
// operation stored ImportMappingCall.ContentType "Json", and mx check refused
// it — "The selected import mapping cannot handle content type JSON." The
// content type is the mapping's, not a constant: it is carried on read and
// written as given.

func importContentTypeOf(t *testing.T, g element.Element) string {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bson.Raw = raw
	rh, ok := doc.Lookup("ResultHandling").DocumentOK()
	if !ok {
		t.Fatal("no ResultHandling")
	}
	imc, ok := rh.Lookup("ImportMappingCall").DocumentOK()
	if !ok {
		t.Fatal("no ImportMappingCall")
	}
	return rawStr(imc, "ContentType")
}

func TestWriteImportMappingCallContentType(t *testing.T) {
	for _, tc := range []struct{ given, want string }{
		{"Xml", "Xml"},
		{"Json", "Json"},
		{"", "Json"}, // nothing known: what every caller wrote before
	} {
		h := &microflows.ResultHandlingMapping{MappingID: "M.IMM", ContentType: tc.given}
		g := importXmlActionToGen(&microflows.ImportXmlAction{ResultHandling: h})
		if got := importContentTypeOf(t, g); got != tc.want {
			t.Errorf("import from mapping, ContentType %q: wrote %q, want %q", tc.given, got, tc.want)
		}
		r := restResultHandlingToGen(h, "out")
		raw, err := (&codec.Encoder{}).Encode(r)
		if err != nil {
			t.Fatal(err)
		}
		var rh bson.Raw = raw
		imc, _ := rh.Lookup("ImportMappingCall").DocumentOK()
		if got := rawStr(imc, "ContentType"); got != tc.want {
			t.Errorf("rest call mapping, ContentType %q: wrote %q, want %q", tc.given, got, tc.want)
		}
	}
}

func TestReadImportMappingCallContentType(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "a-1"},
		{Key: "$Type", Value: "Microflows$ImportXmlAction"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "XmlDocumentVariableName", Value: "latestHttpResponse"},
		{Key: "ResultHandling", Value: bson.D{
			{Key: "$ID", Value: "rh-1"},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "ResultVariableName", Value: "Variable_2"},
			{Key: "ImportMappingCall", Value: bson.D{
				{Key: "$ID", Value: "imc-1"},
				{Key: "$Type", Value: "Microflows$ImportMappingCall"},
				{Key: "ContentType", Value: "Xml"},
				{Key: "ReturnValueMapping", Value: "ExamplesModule.Import_mapping"},
				{Key: "ForceSingleOccurrence", Value: false},
				{Key: "Range", Value: bson.D{{Key: "$Type", Value: "Microflows$ConstantRange"}, {Key: "SingleObject", Value: false}}},
			}},
			{Key: "VariableType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "M.Root"}}},
		}},
	})
	h := readImportRange(t, raw)
	if h.ContentType != "Xml" {
		t.Fatalf("read ContentType = %q, want Xml", h.ContentType)
	}
	g := importXmlActionToGen(&microflows.ImportXmlAction{ResultHandling: h})
	if got := importContentTypeOf(t, g); got != "Xml" {
		t.Errorf("rewrite of a stored Xml import wrote ContentType %q", got)
	}
}
