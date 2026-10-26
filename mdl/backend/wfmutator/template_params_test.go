// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"testing"

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
