// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

const imm1418Prelude = `create json structure MyFirstModule.JSON_D sample '{"code": "NL", "name": "Netherlands"}';
create persistent entity MyFirstModule.CountryD ( Code: string, Name: string );
`

const imm1418Microflow = `create or modify microflow MyFirstModule.MF_D ( $P: string )
begin
  $R = import from mapping MyFirstModule.IMM_D($P);
end;
`

const imm1418Mapping = `create import mapping MyFirstModule.IMM_D with json structure MyFirstModule.JSON_D
{ create MyFirstModule.CountryD { Code = code, Name = name } };
`

// mfUnitByName returns a microflow's unit ID and stored bytes.
func mfUnitByName(t *testing.T, exec *Executor, module, name string) (model.ID, []byte) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == name && h.GetModuleName(h.FindModuleID(m.ContainerID)) == module {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return m.ID, append([]byte(nil), raw...)
		}
	}
	return "", nil
}

// entityTypeEntities collects the Entity of every DataTypes$ObjectType /
// ListType in a stored document, and when set != nil replaces it.
func entityTypeEntities(v any, set *string, out *[]string) any {
	switch x := v.(type) {
	case bson.D:
		typ, _ := afGet(x, "$Type").(string)
		isEntityType := typ == "DataTypes$ObjectType" || typ == "DataTypes$ListType"
		for i := range x {
			if isEntityType && x[i].Key == "Entity" {
				s, _ := x[i].Value.(string)
				*out = append(*out, s)
				if set != nil {
					x[i].Value = *set
				}
				continue
			}
			x[i].Value = entityTypeEntities(x[i].Value, set, out)
		}
		return x
	case bson.A:
		for i := range x {
			x[i] = entityTypeEntities(x[i], set, out)
		}
		return x
	}
	return v
}

// mendixlabs/mxcli#1418, at the layer the symptom lives in: a real project
// write. Exec of a microflow calling a mapping created further down the script
// used to "Create microflow" with VariableType.Entity = "", which mx check
// cannot load (ArgumentNullException in EntityType.set_EntityId).
func TestImportMappingForwardRefExecRefuses1418(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	err := agreeExec(t, exec, "mdl 1;\n"+imm1418Prelude+imm1418Microflow+imm1418Mapping)
	if err == nil || !strings.Contains(err.Error(), "import mapping not found") {
		var got []string
		if _, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_D"); raw != nil {
			var d bson.D
			if uerr := bson.Unmarshal(raw, &d); uerr == nil {
				entityTypeEntities(d, nil, &got)
			}
		}
		t.Fatalf("exec err = %v, want the import mapping refused; stored result entities: %q", err, got)
	}
	if id, _ := mfUnitByName(t, exec, "MyFirstModule", "MF_D"); id != "" {
		t.Fatalf("the refused microflow was written anyway (%s)", id)
	}

	// Control: the same statements in definition order execute.
	if err := agreeExec(t, exec, "mdl 1;\n"+imm1418Mapping+imm1418Microflow); err != nil {
		t.Fatalf("control: mapping-first order failed: %v", err)
	}
	_, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_D")
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var got []string
	entityTypeEntities(d, nil, &got)
	if len(got) != 1 || got[0] != "MyFirstModule.CountryD" {
		t.Fatalf("stored result entities = %q, want [MyFirstModule.CountryD]", got)
	}
}

// A project already written by the old bug: re-running the statement once the
// mapping exists printed "Unchanged microflow" and kept the empty entity,
// because describe prints the broken activity as the very statement that would
// repair it. mdl 0 now rebuilds it (healing the project); mdl 1 refuses with
// the reason instead of reporting Unchanged.
func TestImportMappingEmptyStoredEntityIsNotUnchanged1418(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := agreeExec(t, exec, "mdl 1;\n"+imm1418Prelude+imm1418Mapping+imm1418Microflow); err != nil {
		t.Fatal(err)
	}
	corrupt := func() {
		t.Helper()
		id, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_D")
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		empty := ""
		var got []string
		entityTypeEntities(d, &empty, &got)
		if len(got) != 1 {
			t.Fatalf("expected one entity type in MF_D, found %q", got)
		}
		b, err := bson.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
			t.Fatal(err)
		}
	}

	corrupt()
	out.Reset()
	err := agreeExec(t, exec, "mdl 1;\n"+imm1418Microflow)
	if err == nil || !strings.Contains(err.Error(), "no result entity") {
		t.Fatalf("mdl 1 re-run over a stored empty entity: err = %v, output %q; want a refusal naming the missing result entity", err, out.String())
	}

	out.Reset()
	if err := agreeExec(t, exec, imm1418Microflow); err != nil {
		t.Fatalf("mdl 0 re-run: %v", err)
	}
	_, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_D")
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var got []string
	entityTypeEntities(d, nil, &got)
	if len(got) != 1 || got[0] != "MyFirstModule.CountryD" {
		t.Fatalf("after the mdl 0 re-run the stored result entities are %q, want [MyFirstModule.CountryD] (output %q)", got, out.String())
	}
}
