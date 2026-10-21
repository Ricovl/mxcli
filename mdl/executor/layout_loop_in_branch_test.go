// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The merge after an IF whose branch holds a LOOP must sit right of the loop
// box (mendixlabs/mxcli#1598, case 4). measureLoopStatement left out the
// iterator column the builder reserves on the box's left, so the branch measured
// 100px short and the merge landed inside the box: loop 790..1110, merge
// 1050..1090, and the loop's outgoing line ran right to left into it.
func TestMergeAfterALoopInABranchClearsTheLoopBox(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"loop in then", `
  declare $r boolean = false;
  retrieve $L from MyFirstModule.Item;
  if $Flag then
    loop $i in $L begin set $r = true; end loop;
  else
    set $r = false;
  end if;
  return $r;`},
		{"two activities in the loop", `
  declare $r boolean = false;
  retrieve $L from MyFirstModule.Item;
  if $Flag then
    loop $i in $L begin set $r = true; log info node 'x' 'y'; end loop;
  else
    set $r = false;
  end if;
  return $r;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, tc.body)
			var loop *microflows.LoopedActivity
			var merge *microflows.ExclusiveMerge
			for _, o := range fb.objects {
				switch v := o.(type) {
				case *microflows.LoopedActivity:
					loop = v
				case *microflows.ExclusiveMerge:
					merge = v
				}
			}
			if loop == nil || merge == nil {
				t.Fatalf("want a loop and a merge, got loop=%v merge=%v", loop != nil, merge != nil)
			}
			loopRight := loop.Position.X + loop.Size.Width/2
			mergeLeft := merge.Position.X - MergeSize/2
			if mergeLeft < loopRight {
				t.Errorf("merge spans from x=%d, inside the loop box that ends at x=%d (loop centre %d, width %d)",
					mergeLeft, loopRight, loop.Position.X, loop.Size.Width)
			}
		})
	}
}
