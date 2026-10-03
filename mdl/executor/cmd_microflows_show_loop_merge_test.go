// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#942: a loop's object collection holds the objects of its body but
// none of its flows — Mendix stores every flow of a microflow, the loop body's
// too, in the microflow's own collection. Judged against the loop's collection
// alone, every merge in a loop body had in-degree 0, so the `if` at the end of
// a loop body drew "the merge … joins no decision … re-executing this MDL
// DELETES it" on every describe, a freshly built flow included.
func loopBodyMergeFixture(t *testing.T, orphan bool) *microflows.MicroflowObjectCollection {
	t.Helper()
	obj := func(x int) microflows.BaseMicroflowObject {
		return microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(randomTestID())}, Position: model.Point{X: x, Y: 130}}
	}
	inner := &microflows.MicroflowObjectCollection{}
	split := &microflows.ExclusiveSplit{BaseMicroflowObject: obj(300)}
	change := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: obj(400)}}
	merge := &microflows.ExclusiveMerge{BaseMicroflowObject: obj(500)}
	inner.Objects = append(inner.Objects, split, change, merge)

	root := &microflows.MicroflowObjectCollection{}
	start := &microflows.StartEvent{BaseMicroflowObject: obj(100)}
	loop := &microflows.LoopedActivity{BaseMicroflowObject: obj(200), ObjectCollection: inner}
	end := &microflows.EndEvent{BaseMicroflowObject: obj(700)}
	root.Objects = append(root.Objects, start, loop, end)
	edge := func(from, to microflows.MicroflowObject, value string) {
		f := &microflows.SequenceFlow{
			BaseElement: model.BaseElement{ID: model.ID(randomTestID())},
			OriginID:    from.GetID(), DestinationID: to.GetID(),
		}
		if value != "" {
			f.CaseValue = &microflows.ExpressionCase{Expression: value}
		}
		// Every flow in the microflow's own collection, as Mendix stores it.
		root.Flows = append(root.Flows, f)
	}
	edge(start, loop, "")
	edge(loop, end, "")
	if orphan {
		// Control: the merge has one incoming path and joins nothing.
		edge(split, change, "true")
		edge(change, merge, "")
		other := &microflows.EndEvent{BaseMicroflowObject: obj(450)}
		inner.Objects = append(inner.Objects, other)
		edge(split, other, "false")
	} else {
		// `if … then change …; end if;` as the last statement of the body:
		// the true branch through the change, the false branch straight to
		// the merge, which ends the body.
		edge(split, change, "true")
		edge(change, merge, "")
		edge(split, merge, "false")
	}
	return root
}

func TestDroppedMergeWarnings_LoopBodyMergeJudgedAgainstTheMicroflowsFlows(t *testing.T) {
	root := loopBodyMergeFixture(t, false)
	if got := droppedMergeWarnings(nil, root, labelRejoinMerges(root)); len(got) != 0 {
		t.Errorf("flagged the merge closing an `if` at the end of a loop body: %v", got)
	}
	// Control: a genuinely orphan merge in a loop body still warns.
	root = loopBodyMergeFixture(t, true)
	if got := droppedMergeWarnings(nil, root, labelRejoinMerges(root)); len(got) != 1 {
		t.Errorf("an orphan merge in a loop body: got %d warnings, want 1: %v", len(got), got)
	}
}
