// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1518: an after-startup seed microflow created its objects
// with commit = No and never committed, returned or passed them on. It logged
// "Seeded 20"; the table had 0 rows. check, mx check and the build said nothing.

func act(a microflows.MicroflowAction) *microflows.ActionActivity {
	return &microflows.ActionActivity{Action: a}
}

// createItem is the issue's `$o = create Tmp.Item (Cnt = 1)`.
func createItem(commit microflows.CommitType) *microflows.ActionActivity {
	return act(&microflows.CreateObjectAction{
		EntityQualifiedName: "Tmp.Item",
		OutputVariable:      "o",
		Commit:              commit,
		InitialMembers: []*microflows.MemberChange{
			{AttributeQualifiedName: "Tmp.Item.Cnt", Type: microflows.MemberChangeTypeSet, Value: "1"},
		},
	})
}

func persistentTmp(qn string) bool {
	return qn == "Tmp.Item" || qn == "Tmp.Other"
}

func lintCreated(objects ...microflows.MicroflowObject) []linter.Violation {
	oc := &microflows.MicroflowObjectCollection{Objects: objects}
	return findUnpersistedCreates(oc, testMicroflow(), persistentTmp, NewCreatedObjectNeverPersistedRule())
}

// The issue's repro: create, nothing else, return an unrelated Boolean.
func TestCreatedNeverPersisted_IssueRepro(t *testing.T) {
	got := lintCreated(
		&microflows.StartEvent{},
		act(&microflows.CreateVariableAction{VariableName: "b", DataType: &microflows.BooleanType{}, InitialValue: "true"}),
		createItem(microflows.CommitTypeNo),
		&microflows.EndEvent{ReturnValue: "$b"},
	)
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.RuleID != "CONV020" || v.Severity != linter.SeverityWarning {
		t.Errorf("got %s/%s, want CONV020/warning", v.RuleID, v.Severity)
	}
	if !strings.Contains(v.Message, "$o") || !strings.Contains(v.Message, "Tmp.Item") {
		t.Errorf("message %q should name the variable and the entity", v.Message)
	}
}

// Uses that do not persist or hand the object on: still reported.
func TestCreatedNeverPersisted_NonEscapingUsesStillFlagged(t *testing.T) {
	cases := map[string][]microflows.MicroflowObject{
		"member read in a log message": {
			act(&microflows.LogMessageAction{LogNodeName: "'Seed'", TemplateParameters: []string{"toString($o/Cnt)"}}),
		},
		"change without commit": {
			act(&microflows.ChangeObjectAction{ChangeVariable: "o", Commit: microflows.CommitTypeNo,
				Changes: []*microflows.MemberChange{{AttributeQualifiedName: "Tmp.Item.Cnt", Value: "$o/Cnt + 1"}}}),
		},
		"member read in a split": {
			&microflows.ExclusiveSplit{SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$o/Cnt > 0"}},
		},
		"member read in the return value": {
			&microflows.EndEvent{ReturnValue: "$o / Cnt > 0"},
		},
		"inside a loop": {
			&microflows.LoopedActivity{
				LoopSource:       &microflows.IterableList{ListVariableName: "Rows", VariableName: "Row"},
				ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{createItem(microflows.CommitTypeNo)}},
			},
		},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			objects := extra
			if name != "inside a loop" {
				objects = append([]microflows.MicroflowObject{createItem(microflows.CommitTypeNo)}, extra...)
			}
			if got := lintCreated(objects...); len(got) != 1 {
				t.Errorf("want 1 violation, got %d: %+v", len(got), got)
			}
		})
	}
}

// Every way the object can be persisted or handed on: not reported.
func TestCreatedNeverPersisted_EscapesAreNotFlagged(t *testing.T) {
	cases := map[string]microflows.MicroflowObject{
		"commit":                     act(&microflows.CommitObjectsAction{CommitVariable: "o", WithEvents: true}),
		"change with commit":         act(&microflows.ChangeObjectAction{ChangeVariable: "o", Commit: microflows.CommitTypeYes}),
		"change with commit, no evt": act(&microflows.ChangeObjectAction{ChangeVariable: "o", Commit: microflows.CommitTypeYesWithoutEvents}),
		"returned":                   &microflows.EndEvent{ReturnValue: "$o"},
		"microflow argument": act(&microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{
			Microflow:         "Tmp.SUB_Save",
			ParameterMappings: []*microflows.MicroflowCallParameterMapping{{Parameter: "Tmp.SUB_Save.Item", Argument: "$o"}}}}),
		"java action argument": act(&microflows.JavaActionCallAction{JavaAction: "Tmp.JA_Save",
			ParameterMappings: []*microflows.JavaActionParameterMapping{{Parameter: "Tmp.JA_Save.Item",
				Value: &microflows.BasicCodeActionParameterValue{Argument: "$o"}}}}),
		"javascript action argument": act(&microflows.JavaScriptActionCallAction{JavaScriptAction: "Tmp.JSA_Save",
			ParameterMappings: []*microflows.JavaScriptActionParameterMapping{{Parameter: "Tmp.JSA_Save.Item",
				Value: &microflows.BasicCodeActionParameterValue{Argument: "$o"}}}}),
		"workflow context":     act(&microflows.WorkflowCallAction{Workflow: "Tmp.WF", WorkflowContextVariable: "o"}),
		"export mapping":       act(&microflows.ExportMappingCallAction{MappingID: "m1", SourceVariable: "o", OutputVariable: "json"}),
		"show page argument":   act(&microflows.ShowPageAction{PageName: "Tmp.Item_Edit", PageParameterMappings: []*microflows.PageParameterMapping{{Parameter: "Tmp.Item_Edit.Item", Argument: "$o"}}}),
		"show page passed obj": act(&microflows.ShowPageAction{PageName: "Tmp.Item_Edit", PassedObject: "o"}),
		"added to a list":      act(&microflows.ChangeListAction{ChangeVariable: "Items", Type: microflows.ChangeListTypeAdd, Value: "$o"}),
		"association on another object": act(&microflows.ChangeObjectAction{ChangeVariable: "Order", Commit: microflows.CommitTypeNo,
			Changes: []*microflows.MemberChange{{AssociationQualifiedName: "Tmp.Order_Item", Type: microflows.MemberChangeTypeSet, Value: "$o"}}}),
		"association value of a new object": act(&microflows.CreateObjectAction{EntityQualifiedName: "Tmp.Other", OutputVariable: "p",
			Commit:         microflows.CommitTypeYes,
			InitialMembers: []*microflows.MemberChange{{AssociationQualifiedName: "Tmp.Other_Item", Value: "$o"}}}),
		"deleted":         act(&microflows.DeleteObjectAction{DeleteVariable: "o"}),
		"rolled back":     act(&microflows.RollbackObjectAction{RollbackVariable: "o"}),
		"bare in a split": &microflows.ExclusiveSplit{SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$o != empty"}},
		"case differs":    act(&microflows.CommitObjectsAction{CommitVariable: "O"}),
	}
	for name, use := range cases {
		t.Run(name, func(t *testing.T) {
			if got := lintCreated(createItem(microflows.CommitTypeNo), use); len(got) != 0 {
				t.Errorf("want no violation, got %+v", got)
			}
		})
	}
}

// A commit reached only through an error handler, or on one branch only, is
// still a commit: the rule is flow-insensitive on purpose.
func TestCreatedNeverPersisted_ErrorHandlerCommitCounts(t *testing.T) {
	create := createItem(microflows.CommitTypeNo)
	failing := act(&microflows.MicroflowCallAction{ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
		MicroflowCall: &microflows.MicroflowCall{Microflow: "Tmp.SUB_Fails"}})
	handlerCommit := act(&microflows.CommitObjectsAction{CommitVariable: "o"})
	if got := lintCreated(create, failing, handlerCommit, &microflows.EndEvent{}); len(got) != 0 {
		t.Errorf("want no violation, got %+v", got)
	}
}

// What the rule leaves alone by design.
func TestCreatedNeverPersisted_OutOfScope(t *testing.T) {
	nonPersistent := act(&microflows.CreateObjectAction{EntityQualifiedName: "Tmp.Helper", OutputVariable: "h", Commit: microflows.CommitTypeNo})
	unknownCommit := createItem("")
	committed := createItem(microflows.CommitTypeYes)
	noOutput := act(&microflows.CreateObjectAction{EntityQualifiedName: "Tmp.Item", Commit: microflows.CommitTypeNo})
	for name, obj := range map[string]microflows.MicroflowObject{
		"non-persistent entity":   nonPersistent,
		"commit value not stored": unknownCommit,
		"created with commit":     committed,
		"no output variable":      noOutput,
	} {
		t.Run(name, func(t *testing.T) {
			if got := lintCreated(obj); len(got) != 0 {
				t.Errorf("want no violation, got %+v", got)
			}
		})
	}
}

// Nanoflows and rules are skipped by Check, and without a reader it does nothing.
func TestCreatedNeverPersistedRule_NilReader(t *testing.T) {
	if got := NewCreatedObjectNeverPersistedRule().Check(linter.NewLintContextFromDB(nil)); got != nil {
		t.Errorf("want nil without a reader, got %v", got)
	}
}

// $o must not match $other or $o_2.
func TestCreatedNeverPersisted_WholeVariableNameOnly(t *testing.T) {
	got := lintCreated(createItem(microflows.CommitTypeNo),
		act(&microflows.CommitObjectsAction{CommitVariable: "other"}),
		&microflows.EndEvent{ReturnValue: "$o_2"},
	)
	if len(got) != 1 {
		t.Errorf("want 1 violation, got %d: %+v", len(got), got)
	}
}
