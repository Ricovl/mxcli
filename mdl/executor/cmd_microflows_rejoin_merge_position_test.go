// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The merge an ON ERROR handler falls through to sat on top of the node after
// the guarded activity (mendixlabs/mxcli#884, TDSC0706's repro; ako/mxcli#1273):
//
//	ExclusiveMerge @(560, 200) 40x40
//	EndEvent       @(560, 200) 20x20   <- same centre, the end inside the merge
//
// It was placed at the layout cursor less half a spacing, and the cursor had
// already moved past the successor; with an activity after the guarded one it
// landed RIGHT of that activity, so the edge into it ran backwards. It now sits
// on the edge it splits, between the two boxes.
func TestRejoinMergeSitsBetweenTheGuardedActivityAndItsSuccessor(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"successor is the end event", `
  @position(240, 200)
  call microflow M.SUB_Callee() on error without rollback {
    @position(240, 380)
    log error node 'Repro' 'handled';
  };
  @position(560, 200)
  return;`},
		{"successor is an activity, no positions", `
  call microflow M.SUB_Callee() on error without rollback {
    log error node 'Repro' 'handled';
  };
  log info node 'Repro' 'done';`},
		{"inside an else branch", `
  if $Flag then
    log info node 'Repro' 'then';
  else
    call microflow M.SUB_Callee() on error without rollback {
      log error node 'Repro' 'handled';
    };
    log info node 'Repro' 'after';
  end if;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, tc.body)
			objects := map[model.ID]microflows.MicroflowObject{}
			for _, o := range fb.objects {
				objects[o.GetID()] = o
			}
			var checked int
			for _, f := range fb.flows {
				merge, ok := objects[f.OriginID].(*microflows.ExclusiveMerge)
				if !ok || f.IsErrorHandler {
					continue
				}
				// The merge the handler rejoins at: one normal and one handler
				// flow in, the successor out.
				var fromHandler bool
				var guarded microflows.MicroflowObject
				for _, in := range fb.flows {
					if in.DestinationID != merge.ID {
						continue
					}
					if _, isAct := objects[in.OriginID].(*microflows.ActionActivity); isAct && !in.IsErrorHandler {
						if a := objects[in.OriginID]; a.GetPosition().Y == merge.Position.Y || guarded == nil {
							guarded = a
						}
					}
					if in.IsErrorHandler || objects[in.OriginID].GetPosition().Y > merge.Position.Y {
						fromHandler = true
					}
				}
				if !fromHandler || guarded == nil {
					continue
				}
				next := objects[f.DestinationID]
				mergeL, mergeR := merge.Position.X-MergeSize/2, merge.Position.X+MergeSize/2
				guardedR := guarded.GetPosition().X + objectSize(guarded).Width/2
				nextL := next.GetPosition().X - objectSize(next).Width/2
				if mergeL < guardedR || mergeR > nextL {
					t.Errorf("rejoin merge spans x %d..%d; the guarded activity ends at %d and the %T after it starts at %d",
						mergeL, mergeR, guardedR, next, nextL)
				}
				if merge.Position.Y != next.GetPosition().Y {
					t.Errorf("rejoin merge at y=%d, off the line of the %T it enters (y=%d)",
						merge.Position.Y, next, next.GetPosition().Y)
				}
				checked++
			}
			if checked == 0 {
				t.Fatal("no rejoin merge found")
			}
		})
	}
}
