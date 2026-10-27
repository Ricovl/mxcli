// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 3: a flow call's output mappings (Mendix 11.5, Studio Pro's
// "Return value mappings") had no MDL. describe → exec could only carry the
// stored ones, and a script could not write one. `returns ( … )` spells them;
// these tests hold the stored shape to what Studio Pro writes for each kind of
// row (ako/TestApp Pages.OutputMappingButton, 11.15) and the round trip to a
// no-op.
package executor

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

const flowReturnsPrelude = `create persistent entity MyFirstModule.Vehicle (Brand: String(200));
create microflow MyFirstModule.ACT_Vehicle_Get () returns MyFirstModule.Vehicle
begin
  $Vehicle = create MyFirstModule.Vehicle;
  return $Vehicle;
end;
create microflow MyFirstModule.ACT_String_Get () returns String
begin
  return 'x';
end;
create nanoflow MyFirstModule.NF_Vehicle_Get () returns MyFirstModule.Vehicle
begin
  $Vehicle = create MyFirstModule.Vehicle;
  return $Vehicle;
end;
`

// flowReturnsPage is the TestApp page in MDL: one button per kind of row, plus
// a call with no mappings as the control.
const flowReturnsPage = `create or modify page MyFirstModule.OM_Page (
  Title: 'Output mappings', Layout: Atlas_Core.Atlas_Default,
  Params: ($Vehicle: MyFirstModule.Vehicle),
  Variables: ($stringResult: String = 'empty')
) {
  dataview dvVehicle (DataSource: $Vehicle) {
    actionbutton btnAttributeToAttribute (Caption: 'A', Action: call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = $ActionReturnValue/Brand))
    actionbutton btnToPageVariable (Caption: 'V', Action: call microflow MyFirstModule.ACT_String_Get returns ($stringResult = $ActionReturnValue))
    actionbutton btnExpressionMapping (Caption: 'E', Action: call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = 'Demo brand'))
    actionbutton btnNanoflowMapping (Caption: 'N', Action: call nanoflow MyFirstModule.NF_Vehicle_Get returns (Brand = $ActionReturnValue/Brand))
    actionbutton btnControlNoMapping (Caption: 'C', Action: call microflow MyFirstModule.ACT_Vehicle_Get)
  }
};
`

// openPedAppFixtureAt is openPedAppFixture with the project recorded as made
// by Mendix version: an attribute target needs 11.15 (mx 11.13 and 11.14 report
// CE7006 on it), and PedApp is 11.13.
func openPedAppFixtureAt(t *testing.T, version string) (*Executor, *bytes.Buffer) {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	mpr := filepath.Join(dir, "PedApp.mpr")
	if err := copyPedAppFile(filepath.Join(src, "PedApp.mpr"), mpr); err != nil {
		t.Fatal(err)
	}
	if err := copyPedAppTree(filepath.Join(src, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", mpr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE _MetaData SET _ProductVersion = ?", version); err != nil {
		t.Fatalf("set _ProductVersion: %v", err)
	}
	db.Close()
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	if !strings.Contains(out.String(), "Mendix "+version) {
		t.Fatalf("the fixture did not open as %s:\n%s", version, out.String())
	}
	return exec, out
}

// storedOutputMapping is one Forms$OutputMapping, flattened to the four slots.
type storedOutputMapping struct {
	Button, AttributeRef, SourceAttributeRef, SourceVariable, Expression string
	Keys                                                                 string
}

func attributeOf(v any) string {
	if d, ok := v.(bson.D); ok {
		s, _ := afGet(d, "Attribute").(string)
		return s
	}
	return ""
}

// storedOutputMappings returns every stored Forms$OutputMapping on the page, by
// the button that owns it.
func storedOutputMappings(t *testing.T, exec *Executor, page string) []storedOutputMapping {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []storedOutputMapping
	var walk func(v any, button string)
	walk = func(v any, button string) {
		switch n := v.(type) {
		case bson.D:
			if afGet(n, "$Type") == "Forms$ActionButton" {
				button, _ = afGet(n, "Name").(string)
			}
			if afGet(n, "$Type") == "Forms$OutputMapping" {
				m := storedOutputMapping{Button: button,
					AttributeRef:       attributeOf(afGet(n, "AttributeRef")),
					SourceAttributeRef: attributeOf(afGet(n, "SourceAttributeRef")),
				}
				if sv, ok := afGet(n, "SourceVariable").(bson.D); ok {
					m.SourceVariable, _ = afGet(sv, "LocalVariable").(string)
				}
				m.Expression, _ = afGet(n, "Expression").(string)
				var keys []string
				for _, e := range n {
					keys = append(keys, e.Key)
				}
				sort.Strings(keys) // the set Studio Pro writes; order is not content
				m.Keys = strings.Join(keys, ",")
				out = append(out, m)
			}
			for _, e := range n {
				walk(e.Value, button)
			}
		case bson.A:
			for _, e := range n {
				walk(e, button)
			}
		}
	}
	walk(doc, "")
	return out
}

func TestFlowReturnsWriteStudioProShape(t *testing.T) {
	exec, out := openPedAppFixtureAt(t, "11.15.0")
	if err := afRun(t, exec, "mdl 1;\n"+flowReturnsPrelude+flowReturnsPage); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	got := storedOutputMappings(t, exec, "OM_Page")
	const keys = "$ID,$Type,AttributeRef,Expression,SourceAttributeRef,SourceVariable"
	want := []storedOutputMapping{
		{Button: "btnAttributeToAttribute", AttributeRef: "MyFirstModule.Vehicle.Brand", SourceAttributeRef: "MyFirstModule.Vehicle.Brand", Keys: keys},
		{Button: "btnToPageVariable", SourceVariable: "stringResult", Keys: keys},
		{Button: "btnExpressionMapping", AttributeRef: "MyFirstModule.Vehicle.Brand", Expression: "'Demo brand'", Keys: keys},
		{Button: "btnNanoflowMapping", AttributeRef: "MyFirstModule.Vehicle.Brand", SourceAttributeRef: "MyFirstModule.Vehicle.Brand", Keys: keys},
	}
	if len(got) != len(want) {
		t.Fatalf("stored output mappings:\n%+v\nwant:\n%+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mapping %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}

	// describe prints them, and executing the description is a no-op.
	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.OM_Page;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	for _, s := range []string{
		"call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = $ActionReturnValue/Brand)",
		"call microflow MyFirstModule.ACT_String_Get returns ($stringResult = $ActionReturnValue)",
		"call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = 'Demo brand')",
		"call nanoflow MyFirstModule.NF_Vehicle_Get returns (Brand = $ActionReturnValue/Brand)",
	} {
		if !strings.Contains(described, s) {
			t.Errorf("describe does not print %q:\n%s", s, described)
		}
	}
	if strings.Contains(described, "output mapping") {
		t.Errorf("describe still notes output mappings MDL can now say:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}

	// Control: a statement that drops a returns clause drops the mapping —
	// the statement is the authority, nothing is carried behind its back.
	if err := afRun(t, exec, "mdl 1;\n"+strings.Replace(flowReturnsPage,
		" returns (Brand = 'Demo brand')", "", 1)); err != nil {
		t.Fatalf("exec without one mapping: %v", err)
	}
	if got := storedOutputMappings(t, exec, "OM_Page"); len(got) != 3 {
		t.Errorf("after removing one returns clause the page has %d mappings, want 3: %+v", len(got), got)
	}
}

func TestFlowReturnsAlterSet(t *testing.T) {
	exec, out := openPedAppFixtureAt(t, "11.15.0")
	if err := afRun(t, exec, "mdl 1;\n"+flowReturnsPrelude+flowReturnsPage); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	if err := afRun(t, exec, "alter page MyFirstModule.OM_Page { set (Action: call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = $ActionReturnValue/Brand)) on btnControlNoMapping };"); err != nil {
		t.Fatalf("alter set: %v", err)
	}
	var n int
	for _, m := range storedOutputMappings(t, exec, "OM_Page") {
		if m.Button == "btnControlNoMapping" {
			n++
			if m.AttributeRef != "MyFirstModule.Vehicle.Brand" || m.SourceAttributeRef != "MyFirstModule.Vehicle.Brand" {
				t.Errorf("alter set wrote %+v", m)
			}
		}
	}
	if n != 1 {
		t.Errorf("alter set wrote %d mappings on btnControlNoMapping, want 1", n)
	}
}

func TestFlowReturnsRefusals(t *testing.T) {
	exec, out := openPedAppFixtureAt(t, "11.15.0")
	if err := afRun(t, exec, "mdl 1;\n"+flowReturnsPrelude); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	page := func(action string, inDataView bool) string {
		button := "actionbutton b1 (Caption: 'Go', Action: " + action + ")"
		if inDataView {
			button = "dataview dv (DataSource: $Vehicle) { " + button + " }"
		}
		return "mdl 1;\ncreate or modify page MyFirstModule.R_Page (Title: 'R', Layout: Atlas_Core.Atlas_Default, " +
			"Params: ($Vehicle: MyFirstModule.Vehicle), Variables: ($s: String = 'empty')) { " + button + " };"
	}
	for _, tc := range []struct {
		name, action string
		inDataView   bool
		want         string
	}{
		{"attribute target outside a data container",
			"call microflow MyFirstModule.ACT_Vehicle_Get returns (Brand = $ActionReturnValue/Brand)", false, "data container"},
		{"unknown page variable",
			"call microflow MyFirstModule.ACT_String_Get returns ($nope = $ActionReturnValue)", true, "$nope"},
		{"attribute of a flow that returns no object",
			"call microflow MyFirstModule.ACT_String_Get returns (Brand = $ActionReturnValue/Brand)", true, "returns no object"},
		{"attribute the context entity does not have",
			"call microflow MyFirstModule.ACT_Vehicle_Get returns (Colour = 'red')", true, "Colour"},
	} {
		err := afRun(t, exec, page(tc.action, tc.inDataView))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want a refusal naming %q", tc.name, err, tc.want)
		}
	}
	// Control: the same page with a valid mapping is accepted.
	if err := afRun(t, exec, page("call microflow MyFirstModule.ACT_String_Get returns ($s = $ActionReturnValue)", true)); err != nil {
		t.Errorf("a valid mapping was refused: %v", err)
	}
}

// mx 11.13 and 11.14 report CE7006 on an attribute target that 11.15 accepts,
// byte for byte; a page-variable target checks clean on all three.
func TestFlowReturnsAttributeTargetFloor(t *testing.T) {
	exec, out := openPedAppFixtureAt(t, "11.13.0")
	if err := afRun(t, exec, "mdl 1;\n"+flowReturnsPrelude); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	page := func(returns string) string {
		return "mdl 1;\ncreate or modify page MyFirstModule.R_Page (Title: 'R', Layout: Atlas_Core.Atlas_Default, " +
			"Params: ($Vehicle: MyFirstModule.Vehicle), Variables: ($s: String = 'empty')) { " +
			"dataview dv (DataSource: $Vehicle) { actionbutton b1 (Caption: 'Go', Action: call microflow MyFirstModule." + returns + ") } };"
	}
	err := afRun(t, exec, page("ACT_Vehicle_Get returns (Brand = $ActionReturnValue/Brand)"))
	if err == nil || !strings.Contains(err.Error(), "11.15.0") {
		t.Errorf("an attribute target on 11.13: err = %v, want a refusal naming 11.15.0", err)
	}
	if err := afRun(t, exec, page("ACT_String_Get returns ($s = $ActionReturnValue)")); err != nil {
		t.Errorf("a page-variable target on 11.13 was refused: %v", err)
	}
}
