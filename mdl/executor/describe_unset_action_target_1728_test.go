// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1728: an action whose flow, page or entity is not selected yet —
// the state Studio Pro's page templates ship and a page made from one inherits
// — described as a bare `call nanoflow`, `call microflow`, `show page` or
// `create object`, none of which parses, so one unwired button made the whole
// page's description unusable. mx check reports each as an error (CE0571,
// CE0566, CE0460); MDL has no spelling for it, so describe says so instead.
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

const unsetTargetSetup = `mdl 1;
create persistent entity MyFirstModule.Thing (Name: String(50));
create microflow MyFirstModule.ACT_Go () begin end;
create nanoflow MyFirstModule.NF_Go () begin end;
create page MyFirstModule.Target_Page (Title: 'Target', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: 'x')
};
create page MyFirstModule.Calls_Page (Title: 'Calls', Layout: Atlas_Core.Atlas_Default) {
  actionbutton btnNanoflow (Caption: 'Nanoflow', Action: call nanoflow MyFirstModule.NF_Go with (ProgressBar: Blocking))
  actionbutton btnMicroflow (Caption: 'Microflow', Action: call microflow MyFirstModule.ACT_Go)
  actionbutton btnPage (Caption: 'Page', Action: show page MyFirstModule.Target_Page)
  actionbutton btnCreate (Caption: 'New', Action: create object MyFirstModule.Thing then show page MyFirstModule.Target_Page)
  actionbutton btnControl (Caption: 'Save', Action: save changes)
};
`

// unsetActionTargets blanks what each action on the page targets, the way
// Studio Pro stores an action whose flow, page or entity is not selected.
func unsetActionTargets(t *testing.T, exec *Executor, page string) {
	t.Helper()
	id, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walkStoredDocs(doc, func(d bson.D) {
		for i := range d {
			switch {
			case afGet(d, "$Type") == "Forms$CallNanoflowClientAction" && d[i].Key == "Nanoflow",
				afGet(d, "$Type") == "Forms$MicroflowSettings" && d[i].Key == "Microflow",
				afGet(d, "$Type") == "Forms$FormSettings" && d[i].Key == "Form":
				d[i].Value = ""
			case afGet(d, "$Type") == "Forms$CreateObjectClientAction" && d[i].Key == "EntityRef":
				d[i].Value = nil
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
}

func TestDescribeActionWithUnsetTarget(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, unsetTargetSetup); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	unsetActionTargets(t, exec, "Calls_Page")

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Calls_Page;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if _, errs := visitor.Build(described); len(errs) > 0 {
		t.Fatalf("the description does not parse: %v\n%s", errs, described)
	}
	for _, want := range []string{
		"btnNanoflow", "calls a nanoflow but none is selected (CE0571)",
		"calls a microflow but none is selected (CE0571)",
		"shows a page but none is selected (CE0566)",
		"creates an object but no entity is selected (CE0460)",
		"Action: save changes", // control: a complete action is described as before
	} {
		if !strings.Contains(described, want) {
			t.Errorf("describe does not print %q:\n%s", want, described)
		}
	}
	if strings.Contains(described, "with (ProgressBar") {
		t.Errorf("the settings of an action described as a note were printed on their own:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-executing the description: %v", err)
	}
}

// The two other places a rendered action is embedded print it as `key: value`;
// a note there has to stand on its own line, as it does under `Action:`.
func TestNamedActionNoteStandsAlone(t *testing.T) {
	note := "-- NOT re-executable: x"
	got := appendNamedActionProps(nil, rawWidget{NamedActions: []rawNamedAction{{Key: "onClickAction", MDL: note}}})
	if len(got) != 1 || got[0] != note {
		t.Errorf("named action note = %q, want the note alone", got)
	}
	// Control: an ordinary action keeps its key.
	got = appendNamedActionProps(nil, rawWidget{NamedActions: []rawNamedAction{{Key: "onClickAction", MDL: "close page"}}})
	if len(got) != 1 || got[0] != "onClickAction: close page" {
		t.Errorf("named action = %q", got)
	}
}
