// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 1, as reported from the JTSBootLogboek round trip:
// "`Dashboard_Fullscreen_web` `radioButtons1` stores
// `Forms$RadioButtonGroup.ReadOnlyStyle: Text`; `describe` prints `radiobuttons
// radioButtons1 (Attribute: IsAanwezig)`, and `exec` stores `Inherit`."
//
// The same held for text box, text area and date picker: the writer hard-coded
// Inherit and describe never read the key. Only the check box (#490) and the
// data view (#550) carried it.
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

const ros977Prelude = `create persistent entity MyFirstModule.Crew (Name: String(100), Notes: String(unlimited), Joined: DateTime, Present: Boolean);
create microflow MyFirstModule.DS_Crew () returns MyFirstModule.Crew as $Crew
begin
  $Crew = create MyFirstModule.Crew (Name = 'Ann');
  return $Crew;
end;
`

func ros977Page(name, style string) string {
	set := ""
	if style != "" {
		set = ", ReadOnlyStyle: " + style
	}
	return "create or modify page MyFirstModule." + name + " (Title: 'Crew', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  dataview dvCrew (DataSource: microflow MyFirstModule.DS_Crew) {\n" +
		"    textbox textBox1 (Attribute: Name" + set + ")\n" +
		"    textarea textArea1 (Attribute: Notes" + set + ")\n" +
		"    datepicker datePicker1 (Attribute: Joined" + set + ")\n" +
		"    radiobuttons radioButtons1 (Attribute: Present" + set + ")\n" +
		"  }\n};\n"
}

var ros977Widgets = []string{"textBox1", "textArea1", "datePicker1", "radioButtons1"}

// pageUnitByName returns the id and stored bytes of MyFirstModule.<name>.
func pageUnitByName(t *testing.T, exec *Executor, name string) (model.ID, []byte) {
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
		if p.Name == name && h.GetModuleName(h.FindModuleID(p.ContainerID)) == "MyFirstModule" {
			raw, err := ctx.Backend.GetRawUnitBytes(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			return p.ID, raw
		}
	}
	t.Fatalf("page MyFirstModule.%s not found", name)
	return "", nil
}

// storedReadOnlyStyles maps each named widget on the page to its stored
// ReadOnlyStyle.
func storedReadOnlyStyles(t *testing.T, exec *Executor, page string) map[string]string {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	walkStoredDocs(doc, func(d bson.D) {
		if s, ok := afGet(d, "ReadOnlyStyle").(string); ok {
			if name, _ := afGet(d, "Name").(string); name != "" {
				out[name] = s
			}
		}
	})
	return out
}

func TestInputWidgetReadOnlyStyle_AuthoredIsWritten(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	if err := afRun(t, exec, ros977Prelude+ros977Page("Crew_Authored", "Control")); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := storedReadOnlyStyles(t, exec, "Crew_Authored")
	for _, w := range ros977Widgets {
		if got[w] != "Control" {
			t.Errorf("%s: stored ReadOnlyStyle %q, want Control (all: %v)", w, got[w], got)
		}
	}

	// `alter page … set` was refused on every built-in widget: "property
	// ReadOnlyStyle not found on this widget".
	alter := "alter page MyFirstModule.Crew_Authored {\n"
	for _, w := range ros977Widgets {
		alter += "  set (ReadOnlyStyle: Text) on " + w + "\n"
	}
	if err := afRun(t, exec, alter+"};"); err != nil {
		t.Fatalf("alter page set ReadOnlyStyle: %v", err)
	}
	got = storedReadOnlyStyles(t, exec, "Crew_Authored")
	for _, w := range ros977Widgets {
		if got[w] != "Text" {
			t.Errorf("%s: after alter set, ReadOnlyStyle %q, want Text", w, got[w])
		}
	}
}

// The reported round trip, on a page whose four widgets carry ReadOnlyStyle
// Text as Studio Pro stores it.
func TestInputWidgetReadOnlyStyle_StudioProValueSurvivesDescribeExec(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, ros977Prelude+ros977Page("Crew_Overview", "")); err != nil {
		t.Fatalf("exec: %v", err)
	}
	id, raw := pageUnitByName(t, exec, "Crew_Overview")
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		if _, ok := afGet(d, "ReadOnlyStyle").(string); !ok || afGet(d, "$Type") == "Forms$DataView" {
			return
		}
		for i := range d {
			if d[i].Key == "ReadOnlyStyle" {
				d[i].Value = "Text"
			}
		}
	})
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Crew_Overview;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	for _, w := range ros977Widgets {
		line := ""
		for _, l := range strings.Split(described, "\n") {
			if strings.Contains(l, " "+w+" ") {
				line = l
			}
		}
		if !strings.Contains(line, "ReadOnlyStyle: Text") {
			t.Errorf("describe dropped %s's ReadOnlyStyle: Text: %q", w, line)
		}
	}

	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	got := storedReadOnlyStyles(t, exec, "Crew_Overview")
	for _, w := range ros977Widgets {
		if got[w] != "Text" {
			t.Errorf("%s: after describe → exec ReadOnlyStyle is %q, want Text (all: %v)", w, got[w], got)
		}
	}
}
