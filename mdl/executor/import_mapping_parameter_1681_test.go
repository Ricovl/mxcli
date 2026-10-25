// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1681: an import-with-mapping activity's input object
// (ImportMappingCall.ParameterVariableName) and commit mode had no MDL. describe
// printed `import from mapping JTSBootLogboek.stormglass_import_mapping($stormglass)
// all;`, dropping the object, and v0.28.0 refused the statement under both
// `create or modify` and `create or replace` — naming `create or modify` either
// way, and giving the reason as "no result entity" for an activity that binds no
// result.
//
// Measured on mxbuild 11.13.0 with the #265 mapping (an input parameter):
//
//	no parameter on the call             CE0244 "The import mapping requires a
//	                                     parameter but none is specified in the action."
//	a parameter on a mapping without one CE0246 "The import mapping does not
//	                                     accept a parameter but it is specified in the action."
//	ParameterVariableName set            0 errors, bound or not
//	unbound, VariableType DataTypes$VoidType  0 errors
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const imm1681Prelude = `create persistent entity MyFirstModule.ChunkCollection (Name: String(100));
create persistent entity MyFirstModule.Chunk (Text: String(200));
create association MyFirstModule.Chunk_ChunkCollection from MyFirstModule.Chunk to MyFirstModule.ChunkCollection type Reference;
create json structure MyFirstModule.JSON_Embed sample $${"id": "x", "embeddings": [{"text": "t", "idx": 1}]}$$;
create microflow MyFirstModule.MF_FindChunk ($Collection: MyFirstModule.ChunkCollection, $Index: Integer)
returns MyFirstModule.Chunk
begin
  return empty;
end;
create import mapping MyFirstModule.IMM_Embed with json structure MyFirstModule.JSON_Embed
  parameter MyFirstModule.ChunkCollection
{
  create MyFirstModule.ChunkCollection {
    Name = id,
    find MyFirstModule.Chunk_ChunkCollection/MyFirstModule.Chunk
      by MyFirstModule.MF_FindChunk (Collection: parameter, Index: idx)
      = embeddings { Text = text }
  }
};
create import mapping MyFirstModule.IMM_NoParam with json structure MyFirstModule.JSON_Embed
{ create MyFirstModule.ChunkCollection { Name = id } };
`

func imm1681Flow(name, importLine string) string {
	return "create or modify microflow MyFirstModule." + name +
		" ($Json: String, $Coll: MyFirstModule.ChunkCollection)\nbegin\n  " + importLine + "\nend;\n"
}

// storedImportCall returns the stored ImportMappingCall and result
// VariableType of a microflow's first import activity.
func storedImportCall(t *testing.T, exec *Executor, name string) (imc, varType bson.D) {
	t.Helper()
	_, raw := mfUnitByName(t, exec, "MyFirstModule", name)
	if raw == nil {
		t.Fatalf("microflow MyFirstModule.%s not found", name)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(d, func(x bson.D) {
		if afGet(x, "$Type") == "Microflows$ResultHandling" && imc == nil {
			imc, _ = afGet(x, "ImportMappingCall").(bson.D)
			varType, _ = afGet(x, "VariableType").(bson.D)
		}
	})
	return imc, varType
}

func TestImportMappingParameterAndCommitRoundTrip(t *testing.T) {
	exec, out := openPedAppFixture(t)
	src := "mdl 1;\n" + imm1681Prelude +
		imm1681Flow("MF_Bound", "$R = import from mapping MyFirstModule.IMM_Embed($Json) (Parameter: $Coll, Commit: No) all;") +
		imm1681Flow("MF_Unbound", "import from mapping MyFirstModule.IMM_Embed($Json) (Parameter: $Coll);")
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	for _, tc := range []struct{ flow, commit string }{{"MF_Bound", "No"}, {"MF_Unbound", "YesWithoutEvents"}} {
		imc, _ := storedImportCall(t, exec, tc.flow)
		if got := afGet(imc, "ParameterVariableName"); got != "Coll" {
			t.Errorf("%s: ParameterVariableName = %v, want Coll (CE0244 without it)", tc.flow, got)
		}
		if got := afGet(imc, "Commit"); got != tc.commit {
			t.Errorf("%s: Commit = %v, want %s", tc.flow, got, tc.commit)
		}

		out.Reset()
		if err := afRun(t, exec, "describe microflow MyFirstModule."+tc.flow+";"); err != nil {
			t.Fatal(err)
		}
		described := out.String()
		want := "import from mapping MyFirstModule.IMM_Embed($Json) (Parameter: $Coll"
		if tc.commit == "No" {
			want += ", Commit: No)"
		} else {
			want += ")"
		}
		if !strings.Contains(described, want) {
			t.Fatalf("%s: describe does not carry the settings (%q):\n%s", tc.flow, want, described)
		}
		out.Reset()
		if err := agreeExec(t, exec, described); err != nil {
			t.Fatalf("%s: re-exec of the description: %v", tc.flow, err)
		}
		if !strings.Contains(out.String(), "Unchanged microflow") {
			t.Errorf("%s: re-executing the description was not a no-op:\n%s", tc.flow, out.String())
		}
	}
}

// The two calls mxbuild refuses are refused at exec, naming what to write.
func TestImportMappingParameterMismatchRefused(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	if err := agreeExec(t, exec, "mdl 1;\n"+imm1681Prelude); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ line, want string }{
		{"$R = import from mapping MyFirstModule.IMM_Embed($Json) all;", "CE0244"},
		{"$R = import from mapping MyFirstModule.IMM_NoParam($Json) (Parameter: $Coll) all;", "CE0246"},
	} {
		err := agreeExec(t, exec, "mdl 1;\n"+imm1681Flow("MF_Bad", tc.line))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want a refusal naming %s", tc.line, err, tc.want)
		}
		if id, _ := mfUnitByName(t, exec, "MyFirstModule", "MF_Bad"); id != "" {
			t.Fatalf("%s: the refused microflow was written", tc.line)
		}
	}
}

// An activity that binds no result may store no entity at all — a
// DataTypes$VoidType result, which mxbuild loads at 0 errors. v0.28.0 refused
// every statement over such a flow as "has no result entity, which Mendix
// cannot load". The #1418 shape that cannot be loaded — an object type naming
// no entity — is still refused (TestImportMappingEmptyStoredEntityIsNotUnchanged1418).
func TestImportMappingVoidResultIsNotRefused(t *testing.T) {
	exec, out := openPedAppFixture(t)
	flow := imm1681Flow("MF_Void", "import from mapping MyFirstModule.IMM_Embed($Json) (Parameter: $Coll);")
	if err := agreeExec(t, exec, "mdl 1;\n"+imm1681Prelude+flow); err != nil {
		t.Fatal(err)
	}
	id, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_Void")
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(d, func(x bson.D) {
		if afGet(x, "$Type") != "Microflows$ResultHandling" {
			return
		}
		for i := range x {
			if x[i].Key == "VariableType" {
				vt := x[i].Value.(bson.D)
				x[i].Value = bson.D{{Key: "$ID", Value: afGet(vt, "$ID")}, {Key: "$Type", Value: "DataTypes$VoidType"}}
			}
		}
	})
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}

	// describe → exec, the reported round trip.
	out.Reset()
	if err := afRun(t, exec, "describe microflow MyFirstModule.MF_Void;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, "(Parameter: $Coll)") {
		t.Fatalf("describe dropped the input object:\n%s", described)
	}
	out.Reset()
	if err := agreeExec(t, exec, described); err != nil {
		t.Fatalf("re-exec over a stored VoidType result: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged microflow") {
		t.Errorf("want Unchanged, got:\n%s", out.String())
	}
	if _, vt := storedImportCall(t, exec, "MF_Void"); afGet(vt, "$Type") != "DataTypes$VoidType" {
		t.Errorf("the stored result type was rewritten to %v", afGet(vt, "$Type"))
	}
}
