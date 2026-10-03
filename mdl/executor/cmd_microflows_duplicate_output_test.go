// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Duplicate variable names in a flow are MDL063's (ValidateMicroflow), which
// treats the namespace as flow-wide and skips void action calls. The check-time
// body validator used to report them too, scoped per branch — which mxbuild
// does not do (CE0111 across if/else branches) — so it no longer does (#953).
func TestValidateMicroflowRejectsDuplicateImplicitOutputs(t *testing.T) {
	entityRef := ast.QualifiedName{Module: "Synthetic", Name: "Item"}
	stmt := &ast.CreateMicroflowStmt{
		Body: []ast.MicroflowStatement{
			&ast.CreateObjectStmt{
				Variable:   "Item",
				EntityType: entityRef,
			},
			&ast.RetrieveStmt{
				Variable: "Item",
				Source:   entityRef,
				Limit:    "1",
			},
		},
	}

	got := mdl063(ValidateMicroflow(stmt))
	if len(got) != 1 || !strings.Contains(got[0].Message, "'$Item' is created twice") {
		t.Fatalf("MDL063 = %v, want one duplicate $Item", got)
	}
}

func TestValidateMicroflowRejectsDuplicateCallOutputs(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Body: []ast.MicroflowStatement{
			&ast.CallMicroflowStmt{
				OutputVariable: "Result",
				MicroflowName:  ast.QualifiedName{Module: "Synthetic", Name: "Compute"},
			},
			&ast.CallJavaActionStmt{
				OutputVariable: "Result",
				ActionName:     ast.QualifiedName{Module: "Synthetic", Name: "ComputeInJava"},
			},
		},
	}

	got := mdl063(ValidateMicroflow(stmt))
	if len(got) != 1 || !strings.Contains(got[0].Message, "'$Result' is created twice") {
		t.Fatalf("MDL063 = %v, want one duplicate $Result", got)
	}
}

func TestFormatMicroflowActivitiesWarnsAboutDuplicateModelOutputs(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "start"},
					Position:    model.Point{X: 0, Y: 100},
				},
			},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{
					BaseMicroflowObject: microflows.BaseMicroflowObject{
						BaseElement: model.BaseElement{ID: "first"},
						Position:    model.Point{X: 100, Y: 100},
					},
				},
				Action: &microflows.CreateObjectAction{OutputVariable: "Item", EntityQualifiedName: "Synthetic.Item"},
			},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{
					BaseMicroflowObject: microflows.BaseMicroflowObject{
						BaseElement: model.BaseElement{ID: "second"},
						Position:    model.Point{X: 200, Y: 100},
					},
				},
				Action: &microflows.CreateObjectAction{OutputVariable: "Item", EntityQualifiedName: "Synthetic.Item"},
			},
			&microflows.EndEvent{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "end"},
					Position:    model.Point{X: 300, Y: 100},
				},
			},
		},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "first"},
			{OriginID: "first", DestinationID: "second"},
			{OriginID: "second", DestinationID: "end"},
		},
	}
	lines := formatMicroflowActivities(&ExecContext{}, &microflows.Microflow{ObjectCollection: oc}, nil, nil)
	got := strings.Join(lines, "\n")

	if !strings.Contains(got, "-- WARNING: duplicate output variable $Item") {
		t.Fatalf("describe output missing duplicate warning:\n%s", got)
	}
	if strings.Contains(got, "$Item_2") {
		t.Fatalf("describe output must not invent aliases:\n%s", got)
	}
}

// A flow's names are flow-wide: the same output name in each branch of an
// if/else is CE0111 in mxbuild 11.13.0 (ako/mxcli#962, measured in a
// microflow with a non-void Java call and with a retrieve). This test pinned the
// opposite — branch scoping — until the measurement contradicted it.
func TestFormatMicroflowActivitiesWarnsForExclusiveBranchOutputs(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "start"},
					Position:    model.Point{X: 0, Y: 100},
				},
			},
			&microflows.ExclusiveSplit{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "split"},
					Position:    model.Point{X: 100, Y: 100},
				},
				SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$UsePrimaryPath"},
			},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{
					BaseMicroflowObject: microflows.BaseMicroflowObject{
						BaseElement: model.BaseElement{ID: "then_create"},
						Position:    model.Point{X: 200, Y: 100},
					},
				},
				Action: &microflows.CreateObjectAction{OutputVariable: "Result", EntityQualifiedName: "Synthetic.Item"},
			},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{
					BaseMicroflowObject: microflows.BaseMicroflowObject{
						BaseElement: model.BaseElement{ID: "else_retrieve"},
						Position:    model.Point{X: 200, Y: 200},
					},
				},
				Action: &microflows.RetrieveAction{
					OutputVariable: "Result",
					Source: &microflows.DatabaseRetrieveSource{
						EntityQualifiedName: "Synthetic.Item",
					},
				},
			},
			&microflows.EndEvent{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "then_end"},
					Position:    model.Point{X: 300, Y: 100},
				},
			},
			&microflows.EndEvent{
				BaseMicroflowObject: microflows.BaseMicroflowObject{
					BaseElement: model.BaseElement{ID: "else_end"},
					Position:    model.Point{X: 300, Y: 200},
				},
			},
		},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "split"},
			{OriginID: "split", DestinationID: "then_create", CaseValue: &microflows.ExpressionCase{Expression: "true"}},
			{OriginID: "split", DestinationID: "else_retrieve", CaseValue: &microflows.ExpressionCase{Expression: "false"}},
			{OriginID: "then_create", DestinationID: "then_end"},
			{OriginID: "else_retrieve", DestinationID: "else_end"},
		},
	}
	lines := formatMicroflowActivities(&ExecContext{}, &microflows.Microflow{ObjectCollection: oc}, nil, nil)
	got := strings.Join(lines, "\n")

	if !strings.Contains(got, "-- WARNING: duplicate output variable $Result") {
		t.Fatalf("the same output name in both branches is CE0111 and must be warned:\n%s", got)
	}
}

// act is a small helper for building an ActionActivity that assigns an output var.
func act(id, outVar string, x int) *microflows.ActionActivity {
	a := &microflows.ActionActivity{
		Action: &microflows.CreateObjectAction{OutputVariable: outVar, EntityQualifiedName: "Synthetic.Item"},
	}
	a.ID = model.ID(id)
	a.Position = model.Point{X: x, Y: 100}
	return a
}

func warnsDuplicate(t *testing.T, oc *microflows.MicroflowObjectCollection, name string) bool {
	t.Helper()
	lines := formatMicroflowActivities(&ExecContext{}, &microflows.Microflow{ObjectCollection: oc}, nil, nil)
	return strings.Contains(strings.Join(lines, "\n"), "-- WARNING: duplicate output variable $"+name)
}

// A duplicate that spans a split/merge (one assignment before the split, another
// after the merge) is still on a single path — reachability must flag it. This is
// the correctness case the O(2^b) path-walk used to catch and the reachability
// rewrite must preserve.
func TestFormatMicroflowActivitiesWarnsForDuplicateAcrossMerge(t *testing.T) {
	split := &microflows.ExclusiveSplit{SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$c"}}
	split.ID = "split"
	merge := &microflows.ExclusiveMerge{}
	merge.ID = "merge"
	start := &microflows.StartEvent{}
	start.ID = "start"
	end := &microflows.EndEvent{}
	end.ID = "end"
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, act("a", "X", 50), split, merge, act("c", "X", 400), end},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "a"},
			{OriginID: "a", DestinationID: "split"},
			{OriginID: "split", DestinationID: "merge", CaseValue: &microflows.ExpressionCase{Expression: "true"}},
			{OriginID: "split", DestinationID: "merge", CaseValue: &microflows.ExpressionCase{Expression: "false"}},
			{OriginID: "merge", DestinationID: "c"},
			{OriginID: "c", DestinationID: "end"},
		},
	}
	if !warnsDuplicate(t, oc, "X") {
		t.Fatal("expected duplicate $X across split/merge to be flagged")
	}
}

// High-branch flows must describe in polynomial time. Before the reachability
// rewrite, ~20 sequential empty-then diamonds took ~10s and McCabe-44 flows timed
// out at 300s (issue #710). 120 diamonds is well past that threshold; if this
// regresses to path-enumeration it will not finish.
func TestFormatMicroflowActivitiesHighComplexityCompletes(t *testing.T) {
	const n = 120
	objs := []microflows.MicroflowObject{}
	flows := []*microflows.SequenceFlow{}
	start := &microflows.StartEvent{}
	start.ID = "start"
	objs = append(objs, start)
	prev := model.ID("start")
	for i := 0; i < n; i++ {
		sid := model.ID(fmt.Sprintf("split%d", i))
		mid := model.ID(fmt.Sprintf("merge%d", i))
		sp := &microflows.ExclusiveSplit{SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$c"}}
		sp.ID = sid
		mg := &microflows.ExclusiveMerge{}
		mg.ID = mid
		objs = append(objs, sp, mg)
		flows = append(flows,
			&microflows.SequenceFlow{OriginID: prev, DestinationID: sid},
			&microflows.SequenceFlow{OriginID: sid, DestinationID: mid, CaseValue: &microflows.ExpressionCase{Expression: "true"}},
			&microflows.SequenceFlow{OriginID: sid, DestinationID: mid, CaseValue: &microflows.ExpressionCase{Expression: "false"}},
		)
		prev = mid
	}
	end := &microflows.EndEvent{}
	end.ID = "end"
	objs = append(objs, end)
	flows = append(flows, &microflows.SequenceFlow{OriginID: prev, DestinationID: "end"})
	oc := &microflows.MicroflowObjectCollection{Objects: objs, Flows: flows}

	// Just needs to complete; path enumeration would be 2^120.
	_ = duplicateOutputVariableWarnings(oc, func(any) bool { return false })
}

// A loop body opens no scope either: a non-void Java call inside a loop and
// another of the same name after it is CE0111 (mxbuild 11.13.0, ako/mxcli#962).
// The old reachability walk missed it — the second call does not reach the
// loop. Control: distinct names warn nothing.
func TestFormatMicroflowActivitiesNamesAreFlowWide(t *testing.T) {
	start := &microflows.StartEvent{}
	start.ID = "start"
	end := &microflows.EndEvent{}
	end.ID = "end"
	loop := &microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{act("in", "X", 10)},
	}}
	loop.ID = "loop"
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, loop, act("after", "X", 400), end},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "loop"},
			{OriginID: "loop", DestinationID: "after"},
			{OriginID: "after", DestinationID: "end"},
		},
	}
	if !warnsDuplicate(t, oc, "X") {
		t.Error("an output inside a loop and the same name after it not warned")
	}

	loop.ObjectCollection.Objects = []microflows.MicroflowObject{act("in", "Y", 10)}
	if warnsDuplicate(t, oc, "X") || warnsDuplicate(t, oc, "Y") {
		t.Error("control: distinct names warned")
	}
}
