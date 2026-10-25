// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1681, the writer half: importXmlActionToGen wrote Commit
// YesWithoutEvents and an empty ParameterVariableName on every call, so a
// mapping with an input object could not be called (mxbuild CE0244 "The import
// mapping requires a parameter but none is specified in the action.").
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func encodeImportMappingCall(t *testing.T, rh *microflows.ResultHandlingMapping) bson.D {
	t.Helper()
	g := importXmlActionToGen(&microflows.ImportXmlAction{XmlDocumentVariable: "Json", ResultHandling: rh})
	out, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	return bsonnav.DGetDoc(bsonnav.DGetDoc(d, "ResultHandling"), "ImportMappingCall")
}

func TestImportXmlActionToGen_ParameterAndCommit(t *testing.T) {
	for _, tc := range []struct {
		param, commit         string
		wantParam, wantCommit string
	}{
		{"", "", "", "YesWithoutEvents"}, // control: today's defaults
		{"Coll", "No", "Coll", "No"},
		{"NewStormglass", "Yes", "NewStormglass", "Yes"},
	} {
		imc := encodeImportMappingCall(t, &microflows.ResultHandlingMapping{
			MappingID: "M.IMM", ResultEntityID: "M.E", ParameterVariable: tc.param, Commit: tc.commit,
		})
		if got := bsonnav.DGetString(imc, "ParameterVariableName"); got != tc.wantParam {
			t.Errorf("param %q: ParameterVariableName = %q, want %q", tc.param, got, tc.wantParam)
		}
		if got := bsonnav.DGetString(imc, "Commit"); got != tc.wantCommit {
			t.Errorf("commit %q: Commit = %q, want %q", tc.commit, got, tc.wantCommit)
		}
	}
}
