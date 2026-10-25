// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1680, reported against v0.28.0: "`describe` prints the column's own
// `Attribute: Stormglass_Weer/Hour` with the path, but the template as
// `ContentParams: ({1} = Hour format (dateFormat: DateTime))` without it. `exec`
// resolves `Hour` against the grid's entity" — 13 × CE1613 "The selected
// attribute 'JTSBootLogboek.Weer.Hour' no longer exists".
//
// Fixed after v0.28.0 by the mendixlabs/mxcli#1494 change, which taught the one
// template-parameter reader (extractTextTemplateParameters) the association
// path. Its test covers the path helper alone; this pins the Data grid 2 column
// the report was about, through describe and a re-exec, on a Studio Pro-shaped
// parameter (AttributeRef + IndirectEntityRef).
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedColumnTemplateRefs lists, for every template parameter on the page,
// its AttributeRef.Attribute and the $Type of its EntityRef ("" when null).
func storedColumnTemplateRefs(t *testing.T, exec *Executor, page string) [][2]string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListPages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Name != page || h.GetModuleName(h.FindModuleID(p.ContainerID)) != "MyFirstModule" {
			continue
		}
		raw, err := ctx.Backend.GetRawUnitBytes(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		var out [][2]string
		walkStoredDocs(doc, func(d bson.D) {
			if afGet(d, "$Type") != "Forms$ClientTemplateParameter" {
				return
			}
			ref, _ := afGet(d, "AttributeRef").(bson.D)
			attr, _ := afGet(ref, "Attribute").(string)
			entityRef, _ := afGet(ref, "EntityRef").(bson.D)
			typ, _ := afGet(entityRef, "$Type").(string)
			out = append(out, [2]string{attr, typ})
		})
		return out
	}
	t.Fatalf("page MyFirstModule.%s not found", page)
	return nil
}

func TestDataGrid2ColumnTemplateParamKeepsAssociationPath(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	src := `create persistent entity MyFirstModule.Stormglass (Hour: DateTime);
create persistent entity MyFirstModule.Weer (Plaats: String(100));
create association MyFirstModule.Stormglass_Weer from MyFirstModule.Weer to MyFirstModule.Stormglass;
create page MyFirstModule.Weer_Overview (Title: 'Weer', Layout: Atlas_Core.Atlas_Default) {
  datagrid grid1 (DataSource: database MyFirstModule.Weer) {
    column (Attribute: Stormglass_Weer/Hour, Caption: 'Hour', ShowContentAs: dynamicText, Content: '{1}',
            ContentParams: ({1} = Stormglass_Weer/Hour format (dateFormat: DateTime)))
  }
};`
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	want := [2]string{"MyFirstModule.Stormglass.Hour", "DomainModels$IndirectEntityRef"}
	if got := storedColumnTemplateRefs(t, exec, "Weer_Overview"); len(got) != 1 || got[0] != want {
		t.Fatalf("stored template parameter = %v, want %v (the Studio Pro shape)", got, want)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Weer_Overview;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, "{1} = MyFirstModule.Stormglass_Weer/Hour format (dateFormat: DateTime)") {
		t.Fatalf("describe dropped the template parameter's association path, so exec resolves Hour "+
			"against the grid's entity (CE1613):\n%s", described)
	}

	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	if got := storedColumnTemplateRefs(t, exec, "Weer_Overview"); len(got) != 1 || got[0] != want {
		t.Errorf("after the round trip the parameter is %v, want %v", got, want)
	}
}
