// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1597. A `@curve` on a statement inside a `loop` or `while`
// body passed check and exec and was not stored: the body is built by its own
// flowBuilder, which recorded the curve, and the loop handed its flows to the
// parent without the curves that belong on them. applyFlowCurves runs once, on
// the builder that owns the flows, so a curve left on the sub-builder was never
// applied. The same statement after the loop kept its curve.

// loopBodyIDs returns the IDs of every object inside a loop's collection, at
// any depth, so a test can tell a body flow from a main-line one.
func loopBodyIDs(oc *microflows.MicroflowObjectCollection) map[model.ID]bool {
	ids := map[model.ID]bool{}
	var walk func(objs []microflows.MicroflowObject, inLoop bool)
	walk = func(objs []microflows.MicroflowObject, inLoop bool) {
		for _, o := range objs {
			if inLoop {
				ids[o.GetID()] = true
			}
			if l, ok := o.(*microflows.LoopedActivity); ok && l.ObjectCollection != nil {
				walk(l.ObjectCollection.Objects, true)
			}
		}
	}
	walk(oc.Objects, false)
	return ids
}

// curvedFlows splits the flows carrying the given control vectors by whether
// they leave an activity inside a loop body.
func curvedFlows(oc *microflows.MicroflowObjectCollection, from, to string) (inBody, outside int) {
	body := loopBodyIDs(oc)
	for _, f := range oc.Flows {
		if f.OriginControlVector != from || f.DestinationControlVector != to {
			continue
		}
		if body[f.OriginID] {
			inBody++
		} else {
			outside++
		}
	}
	return inBody, outside
}

func TestLoopBody_KeepsCurve(t *testing.T) {
	src := `mdl 1;
create microflow Probe.ACT_Do ($L: List of Probe.Item)
begin
  loop $I in $L
  begin
    @curve(from: (39, 0), to: (0, 61))
    log info node 'n' 'inside-a';
    log info node 'n' 'inside-b';
  end loop;
  @curve(from: (39, 0), to: (0, 61))
  log info node 'n' 'outside-c';
  log info node 'n' 'outside-d';
end;`
	oc := buildCollectionFromMDL(t, false, src)
	inBody, outside := curvedFlows(oc, "39;0", "0;61")
	if outside != 1 {
		t.Fatalf("%d main-line flows carry the curve, want 1 (the control: outside-c → outside-d)", outside)
	}
	if inBody != 1 {
		t.Errorf("%d loop-body flows carry the curve, want 1 (inside-a → inside-b) — "+
			"the @curve inside the loop was recorded on the body's builder and never applied", inBody)
	}
}

func TestWhileBody_KeepsCurve(t *testing.T) {
	src := `mdl 1;
create microflow Probe.ACT_While ()
begin
  declare $N integer = 0;
  while $N < 3
  begin
    @curve(from: (39, 0), to: (0, 61))
    log info node 'n' 'inside-a';
    log info node 'n' 'inside-b';
    set $N = $N + 1;
  end while;
end;`
	oc := buildCollectionFromMDL(t, false, src)
	if inBody, _ := curvedFlows(oc, "39;0", "0;61"); inBody != 1 {
		t.Errorf("%d while-body flows carry the curve, want 1 (inside-a → inside-b)", inBody)
	}
}

// A loop inside a loop is built by a sub-builder of a sub-builder, so the
// curve has to be handed up twice to reach the builder that applies it.
func TestNestedLoopBody_KeepsCurve(t *testing.T) {
	src := `mdl 1;
create microflow Probe.ACT_Nested ($L: List of Probe.Item)
begin
  loop $I in $L
  begin
    loop $J in $L
    begin
      @curve(from: (39, 0), to: (0, 61))
      log info node 'n' 'inner-a';
      log info node 'n' 'inner-b';
    end loop;
  end loop;
end;`
	oc := buildCollectionFromMDL(t, false, src)
	if inBody, _ := curvedFlows(oc, "39;0", "0;61"); inBody != 1 {
		t.Errorf("%d nested-loop-body flows carry the curve, want 1 (inner-a → inner-b)", inBody)
	}
}
