// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1729: a microflow data source stores its call settings in a
// Forms$MicroflowSettings, as a call-microflow action does, and the writer wrote
// FormValidations All there whatever was stored. ako/TestApp's
// Pages.OutputMappingButton has a data view whose source stores None; describe
// → exec rewrote it as All ("Replaced page"). It is the one setting that
// differs on a data source in the fixtures (44 + 4 sources, every other setting
// at its default), so a data source takes `with (FormValidations: …)`, the
// spelling the action already has.
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

const dsFormValidationsSetup = `mdl 1;
create persistent entity MyFirstModule.Thing (Name: String(50));
create microflow MyFirstModule.DS_Thing () returns MyFirstModule.Thing as $out
begin
  $out = create MyFirstModule.Thing;
  return $out;
end;
create microflow MyFirstModule.DS_Things () returns list of MyFirstModule.Thing as $out
begin
  retrieve $out from MyFirstModule.Thing;
  return $out;
end;
create microflow MyFirstModule.ACT_Go () begin end;
`

const dsFormValidationsPage = `create or modify page MyFirstModule.FV_Page (Title: 'FV', Layout: Atlas_Core.Atlas_Default) {
  dataview dvNone (DataSource: microflow MyFirstModule.DS_Thing with (FormValidations: None)) {
    dynamictext t1 (Content: 'x')
  }
  listview lvWidget (DataSource: microflow MyFirstModule.DS_Things with (FormValidations: Widget)) {
    dynamictext t2 (Content: 'y')
  }
  dataview dvAll (DataSource: microflow MyFirstModule.DS_Thing) {
    dynamictext t3 (Content: 'z')
  }
  actionbutton btnNone (Caption: 'Go', Action: call microflow MyFirstModule.ACT_Go with (FormValidations: None))
};
`

// storedFormValidations maps each named widget to the FormValidations of the
// Forms$MicroflowSettings it holds (its data source's, or its action's).
func storedFormValidations(t *testing.T, exec *Executor, page string) map[string]string {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var walk func(v any, owner string)
	walk = func(v any, owner string) {
		switch n := v.(type) {
		case bson.D:
			if name, _ := afGet(n, "Name").(string); name != "" {
				owner = name
			}
			if afGet(n, "$Type") == "Forms$MicroflowSettings" {
				out[owner], _ = afGet(n, "FormValidations").(string)
			}
			for _, e := range n {
				walk(e.Value, owner)
			}
		case bson.A:
			for _, e := range n {
				walk(e, owner)
			}
		}
	}
	walk(doc, "")
	return out
}

func TestDataSourceFormValidations(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, dsFormValidationsSetup+dsFormValidationsPage); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	want := map[string]string{"dvNone": "None", "lvWidget": "Widget", "dvAll": "All", "btnNone": "None"}
	got := storedFormValidations(t, exec, "FV_Page")
	for w, v := range want {
		if got[w] != v {
			t.Errorf("%s: stored FormValidations %q, want %q (all: %v)", w, got[w], v, got)
		}
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.FV_Page;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	for _, s := range []string{
		"DataSource: microflow MyFirstModule.DS_Thing with (FormValidations: None)",
		"DataSource: microflow MyFirstModule.DS_Things with (FormValidations: Widget)",
		"call microflow MyFirstModule.ACT_Go with (FormValidations: None)",
	} {
		if !strings.Contains(described, s) {
			t.Errorf("describe does not print %q:\n%s", s, described)
		}
	}
	// Control: the default is not printed.
	if strings.Contains(described, "DS_Thing with (FormValidations: All)") {
		t.Errorf("describe printed the default:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("describe → exec was not a no-op:\n%s", out.String())
	}
}

// The reported shape: a data source Studio Pro stored with None, read back.
func TestDataSourceFormValidationsStoredByStudioPro(t *testing.T) {
	exec, out := openPedAppFixture(t)
	page := strings.Replace(dsFormValidationsPage, " with (FormValidations: None))", ")", 1)
	if err := afRun(t, exec, dsFormValidationsSetup+page); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	id, raw := pageUnitByName(t, exec, "FV_Page")
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "$Type") != "Forms$MicroflowSource" {
			return
		}
		ms, _ := afGet(d, "MicroflowSettings").(bson.D)
		if afGet(ms, "Microflow") != "MyFirstModule.DS_Thing" {
			return
		}
		for i := range ms {
			if ms[i].Key == "FormValidations" && afGet(ms, "FormValidations") == "All" {
				ms[i].Value = "None"
				break // only dvNone's; dvAll keeps All
			}
		}
	})
	b, _ := bson.Marshal(doc)
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}
	before := storedFormValidations(t, exec, "FV_Page")

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.FV_Page;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("describe → exec of a stored None was not a no-op:\n%s\n%s", out.String(), described)
	}
	if after := storedFormValidations(t, exec, "FV_Page"); after["dvNone"] != before["dvNone"] || before["dvNone"] != "None" {
		t.Errorf("dvNone FormValidations: stored %q, after describe → exec %q", before["dvNone"], after["dvNone"])
	}
}

// A data source takes FormValidations only: the other settings are an action's.
func TestDataSourceSettingsRefused(t *testing.T) {
	for _, setting := range []string{"ProgressBar: Blocking", "Asynchronous: true", "DisabledDuringExecution: false", "Confirmation: 'Sure?'"} {
		_, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n  dataview dv (DataSource: microflow M.F with (" + setting + ")) { }\n}")
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), "data source") {
			t.Errorf("%s on a data source: errs %v, want a refusal naming the data source", setting, errs)
		}
	}
	// Control: FormValidations is accepted, and a nanoflow source takes none.
	if _, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n  dataview dv (DataSource: microflow M.F with (FormValidations: None)) { }\n}"); len(errs) > 0 {
		t.Errorf("FormValidations on a microflow data source was refused: %v", errs)
	}
}
