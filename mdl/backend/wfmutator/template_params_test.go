// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// stringTemplate is a stored Microflows$StringTemplate with no parameters.
func stringTemplate(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Microflows$StringTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Text", Value: text},
	}
}

func templateExprs(t *testing.T, tmpl bson.D) []string {
	t.Helper()
	var out []string
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(tmpl, "Parameters")) {
		pd := p.(bson.D)
		if got := bsonnav.DGetString(pd, "$Type"); got != "Microflows$TemplateParameter" {
			t.Errorf("parameter $Type = %q, want Microflows$TemplateParameter", got)
		}
		out = append(out, bsonnav.DGetString(pd, "Expression"))
	}
	return out
}

// ako/mxcli#1714: ALTER sets a text's parameters — a user task's task name and
// description, and the workflow's display — and a text set without them has
// none.
func TestWorkflowMutator_SetTemplateParameters(t *testing.T) {
	act := makeWfActivity("Workflows$SingleUserTaskActivity", "Review", "Review")
	act = append(act, bson.E{Key: "TaskName", Value: stringTemplate("Review")}, bson.E{Key: "TaskDescription", Value: stringTemplate("")})
	doc := makeWorkflowDoc(act)
	bsonnav.DSet(doc, "WorkflowName", stringTemplate("WF"))
	m := newMutator(doc)

	if err := m.SetActivityProperty("Review", 0, "task_name", "Review {1}"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTemplateParameters("Review", 0, "task_name", []string{"$WorkflowContext/Title"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTemplateParameters("Review", 0, "description", []string{"$WorkflowContext/Title", "$WorkflowInstance/Name"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTemplateParameters("", 0, "display", []string{"$WorkflowContext/Title"}); err != nil {
		t.Fatal(err)
	}
	actDoc, _ := m.findActivityByCaption("Review", 0)
	if got := bsonnav.DGetString(bsonnav.DGetDoc(actDoc, "TaskName"), "Text"); got != "Review {1}" {
		t.Errorf("task name text = %q", got)
	}
	if got := templateExprs(t, bsonnav.DGetDoc(actDoc, "TaskName")); len(got) != 1 || got[0] != "$WorkflowContext/Title" {
		t.Errorf("task name params = %q", got)
	}
	if got := templateExprs(t, bsonnav.DGetDoc(actDoc, "TaskDescription")); len(got) != 2 || got[1] != "$WorkflowInstance/Name" {
		t.Errorf("task description params = %q", got)
	}
	if got := templateExprs(t, bsonnav.DGetDoc(m.rawData, "WorkflowName")); len(got) != 1 {
		t.Errorf("display params = %q", got)
	}

	// A text set without parameters clears the ones it had.
	if err := m.SetTemplateParameters("Review", 0, "task_name", nil); err != nil {
		t.Fatal(err)
	}
	actDoc, _ = m.findActivityByCaption("Review", 0)
	if got := templateExprs(t, bsonnav.DGetDoc(actDoc, "TaskName")); len(got) != 0 {
		t.Errorf("cleared task name params = %q", got)
	}
}

// ako/mxcli#1716: ALTER's insert writes a non-interrupting timer's recurrence,
// and refuses one on an interrupting timer.
func TestWorkflowMutator_InsertBoundaryEvent_Recurrence(t *testing.T) {
	act := makeWfActivity("Workflows$UserTask", "Review", "task1")
	act = append(act, bson.E{Key: "BoundaryEvents", Value: bson.A{int32(3)}})
	m := newMutator(makeWorkflowDoc(act))
	r := &workflows.Recurrence{IntervalType: "Week", Interval: 1, MaxExecutions: 4}
	if err := m.InsertBoundaryEvent("Review", 0, "NonInterruptingTimer", "PT1H", "", r, nil); err != nil {
		t.Fatal(err)
	}
	actDoc, _ := m.findActivityByCaption("Review", 0)
	ev := bsonnav.DGetArrayElements(bsonnav.DGet(actDoc, "BoundaryEvents"))[0].(bson.D)
	rec := bsonnav.DGetDoc(ev, "Recurrence")
	if rec == nil || bsonnav.DGetString(rec, "$Type") != "Workflows$LinearRecurrence" || bsonnav.DGetString(rec, "IntervalType") != "Week" {
		t.Errorf("recurrence = %v", rec)
	}
	if err := m.InsertBoundaryEvent("Review", 0, "InterruptingTimer", "PT1H", "", r, nil); err == nil {
		t.Error("a repeating interrupting timer must be refused")
	}
}
