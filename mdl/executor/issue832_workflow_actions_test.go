// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/rand"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// ako/mxcli#832, the part the pluggable patch left: on ako/TestApp, re-running
// `describe page WorkflowCommons.TaskInbox` unchanged turned the link button in
// each task grid's last column from Studio Pro's "Show user task page"
// (Forms$OpenUserTaskClientAction) into a Forms$NoAction. describe printed no
// action for it, and a classic widget is rebuilt from the statement, so the
// button that opens a task stopped doing anything — check, exec and mx check
// all clean.
//
// The workflow client actions beside it had the same gap: "Show workflow admin
// page" (Forms$OpenWorkflowClientAction) and "Call workflow"
// (Forms$CallWorkflowClientAction) printed nothing either, and `complete task`
// rebuilt its ClosePage and Commit as true whatever was stored.
//
// The seeds are what Studio Pro stores and mxcli's own writer would not: a
// test seeded with the writer's defaults passes against a writer that drops them.

const issue832ActionsSetup = `create persistent entity MyFirstModule.Request (Title: String(100));
create workflow MyFirstModule.WF parameter $WorkflowContext: MyFirstModule.Request begin end workflow;
create page MyFirstModule.TaskActions (Title: 'Task actions', Layout: Atlas_Core.Atlas_Default,
    Params: ($Task: System.WorkflowUserTask, $Wf: System.Workflow, $Request: MyFirstModule.Request)) {
  dataview dvTask (DataSource: $Task) {
    linkbutton btnOpenTask (Caption: 'Open task')
    actionbutton btnDone (Caption: 'Done', Action: complete task 'Done')
  }
  dataview dvWf (DataSource: $Wf) {
    linkbutton btnOpenWf (Caption: 'Admin page')
  }
  dataview dvRequest (DataSource: $Request) {
    actionbutton btnStart (Caption: 'Start')
  }
};`

const issue832InboxSetup = `create page MyFirstModule.Inbox (Title: 'Inbox', Layout: Atlas_Core.Atlas_Default) {
  datagrid dgTasks (DataSource: database from System.WorkflowUserTask) {
    column (Attribute: Name, Caption: 'Task name')
    column (Attribute: Name, Caption: ' ', ShowContentAs: customContent) {
      linkbutton btnRow (Caption: 'Open')
    }
  }
};`

// clientAction is a stored client action of type typ with its own $ID.
func clientAction(typ string, kv ...bson.E) bson.D {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		panic(err)
	}
	d := bson.D{
		{Key: "$ID", Value: bson.Binary{Data: id}},
		{Key: "$Type", Value: typ},
	}
	return append(d, kv...)
}

// seedAction replaces the stored action of the classic widget named name.
func seedAction(t *testing.T, doc bson.D, name string, action bson.D) {
	t.Helper()
	w := namedDoc(t, doc, name)
	if bsonDocField(w, "Action") == nil {
		t.Fatalf("%s stores no Action", name)
	}
	bsonnav.DSet(w, "Action", action)
}

// storedAction is the stored action of the classic widget named name.
func storedAction(t *testing.T, exec *Executor, id, name string) bson.D {
	t.Helper()
	return bsonDocField(namedDoc(t, storedUnitDoc(t, exec, id), name), "Action")
}

// wantAction reports each key of want that the stored action does not hold.
func wantAction(t *testing.T, what string, got bson.D, want bson.D) {
	t.Helper()
	for _, e := range want {
		if e.Key == "$ID" {
			continue
		}
		if v := bsonValue(got, e.Key); v != e.Value {
			t.Errorf("%s: %s = %v, want %v (stored action: %v)", what, e.Key, v, e.Value, got)
		}
	}
}

// reDescribe runs describe's own output for what, unchanged.
func reDescribe(t *testing.T, exec *Executor, out interface {
	Reset()
	String() string
}, what string) string {
	t.Helper()
	out.Reset()
	run(t, exec, "describe "+what)
	described := out.String()
	out.Reset()
	run(t, exec, described)
	return described
}

func TestIssue832_WorkflowClientActionsSurviveARewrite(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832ActionsSetup)
	id := unitIDByName(t, exec, "page", "TaskActions")

	seeds := map[string]bson.D{
		// Not Studio Pro's defaults (true, false), so a writer that ignores
		// what describe printed cannot pass by writing its constants.
		"btnOpenTask": clientAction("Forms$OpenUserTaskClientAction",
			bson.E{Key: "AssignOnOpen", Value: false},
			bson.E{Key: "DisabledDuringExecution", Value: true},
			bson.E{Key: "OpenWhenAssigned", Value: true}),
		"btnDone": clientAction("Forms$SetTaskOutcomeClientAction",
			bson.E{Key: "ClosePage", Value: false},
			bson.E{Key: "Commit", Value: false},
			bson.E{Key: "DisabledDuringExecution", Value: true},
			bson.E{Key: "OutcomeValue", Value: "Done"}),
		"btnOpenWf": clientAction("Forms$OpenWorkflowClientAction",
			bson.E{Key: "DefaultPage", Value: "MyFirstModule.TaskActions"},
			bson.E{Key: "DisabledDuringExecution", Value: false}),
		"btnStart": clientAction("Forms$CallWorkflowClientAction",
			bson.E{Key: "ClosePage", Value: false},
			bson.E{Key: "ConfirmationInfo", Value: nil},
			bson.E{Key: "DisabledDuringExecution", Value: true},
			bson.E{Key: "Workflow", Value: "MyFirstModule.WF"}),
	}
	doc := storedUnitDoc(t, exec, id)
	for name, action := range seeds {
		seedAction(t, doc, name, action)
	}
	writeUnitDoc(t, exec, id, doc)

	described := reDescribe(t, exec, out, "page MyFirstModule.TaskActions")
	for name, want := range seeds {
		wantAction(t, "re-running describe unchanged, "+name, storedAction(t, exec, id, name), want)
	}
	if t.Failed() {
		t.Logf("describe printed:\n%s", described)
	}
}

// The TestApp shape: the link button sits in a data grid column's content, and
// the statement changes another column, so the grid is patched and its column
// content rebuilt from the statement.
func TestIssue832_LinkButtonInAGridColumnKeepsItsUserTaskAction(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832InboxSetup)
	id := unitIDByName(t, exec, "page", "Inbox")

	want := clientAction("Forms$OpenUserTaskClientAction",
		bson.E{Key: "AssignOnOpen", Value: true},
		bson.E{Key: "DisabledDuringExecution", Value: true},
		bson.E{Key: "OpenWhenAssigned", Value: false})
	doc := storedUnitDoc(t, exec, id)
	seedAction(t, doc, "btnRow", want)
	writeUnitDoc(t, exec, id, doc)

	reDescribe(t, exec, out, "page MyFirstModule.Inbox")
	wantAction(t, "re-running describe unchanged", storedAction(t, exec, id, "btnRow"), want)

	run(t, exec, describedThenEdited(t, exec, out, "page MyFirstModule.Inbox", "Caption: 'Task name'", "Caption: 'Renamed task'"))
	if !storedHas(t, exec, id, "Renamed task") {
		t.Fatal("the rewrite did not land, so it shows nothing about the action")
	}
	wantAction(t, "changing another column's caption", storedAction(t, exec, id, "btnRow"), want)
}

// Every spelling and setting describe prints is one exec reads back to the
// same document.
func TestIssue832_WorkflowActionSpellingsRoundTrip(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832ActionsSetup)
	actions := []string{
		"open user task with (AssignOnOpen: false, OpenWhenAssigned: true)",
		"complete task 'Done' with (ClosePage: false, Commit: false)",
		"open workflow with (DisabledDuringExecution: false, DefaultPage: MyFirstModule.TaskActions)",
		"call workflow MyFirstModule.WF with (ClosePage: false, Confirmation: 'Start it?', ProceedCaption: 'Start', CancelCaption: 'Not now')",
	}
	run(t, exec, `create page MyFirstModule.Spelled (Title: 'Spelled', Layout: Atlas_Core.Atlas_Default,
    Params: ($Task: System.WorkflowUserTask, $Wf: System.Workflow, $Request: MyFirstModule.Request)) {
  dataview dvTask (DataSource: $Task) {
    linkbutton b1 (Caption: 'Open', Action: `+actions[0]+`)
    actionbutton b2 (Caption: 'Done', Action: `+actions[1]+`)
  }
  dataview dvWf (DataSource: $Wf) {
    linkbutton b3 (Caption: 'Admin', Action: `+actions[2]+`)
  }
  dataview dvRequest (DataSource: $Request) {
    actionbutton b4 (Caption: 'Start', Action: `+actions[3]+`)
  }
};`)
	id := unitIDByName(t, exec, "page", "Spelled")
	wantAction(t, "call workflow", storedAction(t, exec, id, "b4"), clientAction("Forms$CallWorkflowClientAction",
		bson.E{Key: "ClosePage", Value: false},
		bson.E{Key: "Workflow", Value: "MyFirstModule.WF"}))
	if got := storedAction(t, exec, id, "b4"); bsonValue(got, "Commit") != nil {
		t.Errorf("call workflow wrote Commit, which Mendix 9.12 removed: %v", got)
	}
	wantAction(t, "open workflow", storedAction(t, exec, id, "b3"), clientAction("Forms$OpenWorkflowClientAction",
		bson.E{Key: "DefaultPage", Value: "MyFirstModule.TaskActions"},
		bson.E{Key: "DisabledDuringExecution", Value: false}))

	described := reDescribe(t, exec, out, "page MyFirstModule.Spelled")
	for _, a := range actions {
		if !strings.Contains(described, "Action: "+a) {
			t.Errorf("describe does not print %q:\n%s", a, described)
		}
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-running describe's output rewrote the page:\n%s", out.String())
	}
}

// A stored "Show workflow admin page" with no page is Studio Pro's unfinished
// placeholder (CE1834). `open workflow` without one does not parse, so describe
// says so in a note rather than printing a statement that cannot run.
func TestIssue832_OpenWorkflowWithoutAPageIsANote(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue832ActionsSetup)
	id := unitIDByName(t, exec, "page", "TaskActions")
	doc := storedUnitDoc(t, exec, id)
	seedAction(t, doc, "btnOpenWf", clientAction("Forms$OpenWorkflowClientAction",
		bson.E{Key: "DefaultPage", Value: ""},
		bson.E{Key: "DisabledDuringExecution", Value: true}))
	writeUnitDoc(t, exec, id, doc)

	out.Reset()
	run(t, exec, "describe page MyFirstModule.TaskActions")
	described := out.String()
	if !strings.Contains(described, "NOT re-executable: the action shows a workflow admin page but no default page is selected (CE1834)") {
		t.Errorf("describe does not flag the page-less action:\n%s", described)
	}
	run(t, exec, described) // the description still parses and runs
}

// A fresh `open user task` writes what Studio Pro stores for a new "Show user
// task page" action: auto-assign on, only the assignee may open it.
func TestIssue832_OpenUserTaskWritesStudioProDefaults(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, `create page MyFirstModule.OneTask (Title: 'Task', Layout: Atlas_Core.Atlas_Default, Params: ($Task: System.WorkflowUserTask)) {
  dataview dvTask (DataSource: $Task) {
    linkbutton btnOpenTask (Caption: 'Open task', Action: open user task)
    actionbutton btnDone (Caption: 'Done', Action: complete task 'Done')
  }
};`)
	id := unitIDByName(t, exec, "page", "OneTask")
	wantAction(t, "open user task", storedAction(t, exec, id, "btnOpenTask"), clientAction("Forms$OpenUserTaskClientAction",
		bson.E{Key: "AssignOnOpen", Value: true},
		bson.E{Key: "DisabledDuringExecution", Value: true},
		bson.E{Key: "OpenWhenAssigned", Value: false}))
	wantAction(t, "complete task", storedAction(t, exec, id, "btnDone"), clientAction("Forms$SetTaskOutcomeClientAction",
		bson.E{Key: "ClosePage", Value: true},
		bson.E{Key: "Commit", Value: true}))

	out.Reset()
	run(t, exec, "describe page MyFirstModule.OneTask")
	if got := out.String(); !strings.Contains(got, "Action: open user task") || strings.Contains(got, "AssignOnOpen") {
		t.Errorf("describe should print the defaults as a bare `open user task`:\n%s", got)
	}
}
