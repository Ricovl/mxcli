// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// An IF/ELSE that ends a loop body joins nothing: both branches end the
// iteration. The builder still drew a merge for them, with no way out
// (mendixlabs/mxcli#884, ako/mxcli#1273) — a node Studio Pro's authors delete
// by hand, and which every re-run put back. A loop-body activity with no
// outgoing flow ends the iteration; two of them are two such activities.

func loopBody(t *testing.T, fb *flowBuilder) *microflows.MicroflowObjectCollection {
	t.Helper()
	for _, o := range fb.objects {
		if l, ok := o.(*microflows.LoopedActivity); ok {
			return l.ObjectCollection
		}
	}
	t.Fatal("no loop built")
	return nil
}

func countMergesIn(oc *microflows.MicroflowObjectCollection) int {
	n := 0
	for _, o := range oc.Objects {
		if _, ok := o.(*microflows.ExclusiveMerge); ok {
			n++
		}
	}
	return n
}

func TestIfElseEndingALoopBodyHasNoMerge(t *testing.T) {
	for _, tc := range []struct{ name, loop string }{
		{"loop", "loop $i in $L begin\n%s\nend loop;"},
		{"while", "while $Flag begin\n%s\nend while;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `    log info node 'x' 'first';
    if $Other then
      log info node 'x' 'then';
    else
      log info node 'x' 'else';
    end if;`
			fb := buildFlowFromMDL(t, "  retrieve $L from M.Item;\n  "+strings.Replace(tc.loop, "%s", body, 1))
			oc := loopBody(t, fb)
			if n := countMergesIn(oc); n != 0 {
				t.Errorf("the IF ending the %s body has %d merge(s), want none", tc.name, n)
			}
			// Both branch activities end the iteration: no flow leaves them.
			out := map[model.ID]int{}
			for _, f := range fb.flows {
				out[f.OriginID]++
			}
			branches := 0
			for _, o := range oc.Objects {
				if _, ok := o.(*microflows.ExclusiveSplit); !ok {
					continue
				}
				for _, f := range fb.flows {
					if f.OriginID == o.GetID() {
						branches++
						if out[f.DestinationID] != 0 {
							t.Errorf("a branch activity has %d outgoing flow(s), want 0", out[f.DestinationID])
						}
					}
				}
			}
			if branches != 2 {
				t.Errorf("%d branch flows, want 2", branches)
			}
		})
	}
}

// Nested: the merge-less tail reaches into each branch, so an IF/ELSE that ends
// a branch of the tail IF has no merge either.
func TestNestedIfElseAtTheLoopTailHasNoMerge(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  retrieve $L from M.Item;
  loop $i in $L begin
    if $A then
      if $B then log info node 'x' 'ab'; else log info node 'x' 'a'; end if;
    else
      log info node 'x' 'none';
    end if;
  end loop;`)
	if n := countMergesIn(loopBody(t, fb)); n != 0 {
		t.Errorf("%d merge(s) at the loop tail, want none", n)
	}
}

// Controls: a merge that joins something stays. An IF that is not last, an IF
// with an empty branch (the split needs a flow per case), and an explicit
// @merge, which is how DESCRIBE keeps a stored tail merge through a rebuild.
func TestIfInALoopBodyKeepsAMergeThatJoinsSomething(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"not last", `
    if $Other then log info node 'x' 'then'; else log info node 'x' 'else'; end if;
    log info node 'x' 'after';`},
		{"empty else", `
    if $Other then log info node 'x' 'then'; else end if;`},
		{"no else", `
    if $Other then log info node 'x' 'then'; end if;`},
		{"explicit @merge", `
    @merge(600, 80)
    if $Other then log info node 'x' 'then'; else log info node 'x' 'else'; end if;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, "  retrieve $L from M.Item;\n  loop $i in $L begin"+tc.body+"\n  end loop;")
			if n := countMergesIn(loopBody(t, fb)); n != 1 {
				t.Errorf("%d merge(s), want 1", n)
			}
		})
	}
}

// A tail merge that IS stored (an earlier mxcli wrote one, or a person kept
// it) is described with @merge, so a rebuild from the description keeps it.
func TestDescribeKeepsAStoredLoopTailMerge(t *testing.T) {
	const src = `create microflow M.Tail ($Other: Boolean) returns Boolean
begin
  retrieve $L from M.Item;
  loop $i in $L begin
    @merge(600, 80)
    if $Other then log info node 'x' 'then'; else log info node 'x' 'else'; end if;
  end loop;
  return true;
end;`
	out, oc := describeBuilt(t, src)
	if !strings.Contains(out, "@merge(") {
		t.Errorf("a stored loop-tail merge is described without @merge:\n%s", out)
	}
	if got, want := countMerges(rebuildDescription(t, out)), countMerges(oc); got != want {
		t.Errorf("the description rebuilds %d merges, the flow has %d:\n%s", got, want, out)
	}
}

// The same, through the canonical describe that omits derivable positions: a
// tail merge an earlier mxcli wrote stands where the old layout put it, so its
// position is "derived" — but the current layout draws no merge there at all.
func TestCanonicalDescribeKeepsAStoredLoopTailMerge(t *testing.T) {
	stored := buildMicroflowFromMDL(t, `create microflow M.Tail ($Other: Boolean) returns Boolean
begin
  retrieve $L from M.Item;
  loop $i in $L begin
    @merge(600, 80)
    if $Other then log info node 'x' 'then'; else log info node 'x' 'else'; end if;
  end loop;
  return true;
end;`)
	mf := &microflows.Microflow{ObjectCollection: stored}
	e := newTestExecutor()
	ctx := e.newExecContext(t.Context())
	// A keep set that keeps nothing: every position reads as derivable.
	ctx.describeLayout = newFlowLayoutKeep()
	out := strings.Join(formatMicroflowActivities(ctx, mf, nil, nil), "\n")
	if !strings.Contains(out, "@merge(") {
		t.Errorf("a stored loop-tail merge is described without @merge when positions are derived:\n%s", out)
	}
}

// The tail belongs to the last statement only. A CASE at the tail did not
// clear it, so an IF/ELSE inside one of its arms — followed by another
// statement — lost its merge and the statement after it lost its way in
// (02c-complex-layout-examples.mdl, CX_ACT_BatchProcessFactories).
func TestIfInsideACaseAtTheLoopTailKeepsItsMerge(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  retrieve $L from M.Item;
  loop $i in $L begin
    case $E
      when A then
        if $Other then log info node 'x' 'then'; else log info node 'x' 'else'; end if;
        log info node 'x' 'after';
      when (empty) then
        log info node 'x' 'empty';
    end case;
  end loop;`)
	oc := loopBody(t, fb)
	in := map[model.ID]int{}
	for _, f := range fb.flows {
		in[f.DestinationID]++
	}
	for _, o := range oc.Objects {
		if _, ok := o.(*microflows.ActionActivity); ok && in[o.GetID()] == 0 {
			t.Errorf("an activity at %v has no incoming flow", o.GetPosition())
		}
	}
}
