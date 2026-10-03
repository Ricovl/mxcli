// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1267: every row's Caption was the placeholder "Activity",
// Description was empty, and none of the properties a quality rule checks
// (error handling, log level and node, split condition, commit and events,
// retrieve source) reached the table. TestTestAppActivityProperties measures
// the same columns on Studio Pro-authored BSON; this covers the values TestApp
// has no instance of.
func TestActivityPropertyColumns(t *testing.T) {
	split := loopObj("split", &microflows.ExclusiveSplit{
		Caption:        "Is big?",
		Documentation:  "Routes large orders",
		SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$Order/Amount > 10"},
	})
	loop := loopObj("loop", &microflows.LoopedActivity{
		Documentation:     "One pass per order",
		ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
	})
	create := loopAction("create", &microflows.CreateObjectAction{
		EntityQualifiedName: "Loops.Order", Commit: microflows.CommitTypeYesWithoutEvents,
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	})
	create.Caption = "Make order"
	create.Documentation = "Creates the order"
	change := loopAction("change", &microflows.ChangeObjectAction{Commit: microflows.CommitTypeYes})
	commit := loopAction("commit", &microflows.CommitObjectsAction{WithEvents: false})
	retrieve := loopAction("retrieve", &microflows.RetrieveAction{
		Source: &microflows.DatabaseRetrieveSource{EntityQualifiedName: "Loops.Order"},
	})
	logAct := loopAction("log", &microflows.LogMessageAction{
		LogLevel: microflows.LogLevelWarning, LogNodeName: "@Loops.LogNode",
		MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "Amount is {1}"}},
	})
	note := loopObj("note", &microflows.Annotation{Caption: "Keep this short"})

	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "MF_Props", ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			loopObj("start", &microflows.StartEvent{}),
			split, loop, create, change, commit, retrieve, logAct, note,
			loopObj("end", &microflows.EndEvent{}),
		},
	}}
	mf.ID = "mf-props"
	cat := buildFlowsForTest(t, []*microflows.Microflow{mf}, nil, nil)

	res, err := cat.Query(`SELECT Id, Caption, Description, ConditionExpression, ErrorHandlingType,
		CommitType, WithEvents, RetrieveSource, EntityRef, LogLevel, LogNodeExpression, LogMessage
		FROM activities WHERE MicroflowQualifiedName = 'Loops.MF_Props'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got := map[string][]any{}
	for _, r := range res.Rows {
		got[r[0].(string)] = r[1:]
	}
	want := map[string][]any{
		"start":    {"", "", "", "", "", int64(0), "", "", "", "", ""},
		"split":    {"Is big?", "Routes large orders", "$Order/Amount > 10", "", "", int64(0), "", "", "", "", ""},
		"loop":     {"", "One pass per order", "", "Continue", "", int64(0), "", "", "", "", ""},
		"create":   {"Make order", "Creates the order", "", "Rollback", "YesWithoutEvents", int64(0), "", "Loops.Order", "", "", ""},
		"change":   {"", "", "", "", "Yes", int64(1), "", "", "", "", ""},
		"commit":   {"", "", "", "", "", int64(0), "", "", "", "", ""},
		"retrieve": {"", "", "", "", "", int64(0), "database", "Loops.Order", "", "", ""},
		"log":      {"", "", "", "", "", int64(0), "", "", "Warning", "@Loops.LogNode", "Amount is {1}"},
		"note":     {"Keep this short", "", "", "", "", int64(0), "", "", "", "", ""},
	}
	cols := []string{"Caption", "Description", "ConditionExpression", "ErrorHandlingType", "CommitType",
		"WithEvents", "RetrieveSource", "EntityRef", "LogLevel", "LogNodeExpression", "LogMessage"}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("no row for %s", id)
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s.%s = %#v, want %#v", id, cols[i], g[i], w[i])
			}
		}
	}
}
