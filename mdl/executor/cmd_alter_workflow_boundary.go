// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// boundaryDropAmbiguous is the language change for `drop <activity> boundary
// event` on an activity holding several: the op names no event, and the
// mutators drop the first stored one — a guess at which the author meant
// (found checking the siblings of ako/mxcli#791). Under mdl 1 it is refused;
// under mdl 0 the first is still dropped, with this warning (ADR-0011: a new
// refusal applies only under the header that opts into it).
var boundaryDropAmbiguous = langver.Change{
	Code:  "MDL-V1-BOUNDARYDROP",
	Since: langver.V1,
	Old: "`drop <activity> boundary event` on an activity with several boundary events drops the first one " +
		"`describe workflow` prints",
	New: "a refusal, with nothing written",
}

// checkBoundaryEventDrops applies boundaryDropAmbiguous to every drop boundary
// event op whose stored target holds more than one. A target the stored
// workflow does not settle (not found, ambiguous, inserted by this statement)
// is left to the mutator.
func checkBoundaryEventDrops(ctx *ExecContext, s *ast.AlterWorkflowStmt) error {
	var wf *workflows.Workflow
	for _, op := range s.Operations {
		o, ok := op.(*ast.DropBoundaryEventOp)
		if !ok {
			continue
		}
		if wf == nil {
			if wf = findStoredWorkflow(ctx, s.Name); wf == nil || wf.Flow == nil {
				return nil
			}
		}
		n := len(storedBoundaryEvents(resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition)))
		if n < 2 {
			continue
		}
		if boundaryDropAmbiguous.Applies(ctx.LanguageVersion) {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"alter workflow %s: `drop %s boundary event` is refused: '%s' has %d boundary events, and the "+
					"statement does not say which. Nothing was written. Restate the workflow with "+
					"`create or modify workflow`, keeping the events to keep",
				s.Name, o.ActivityRef, o.ActivityRef, n))
		}
		fmt.Fprintf(ctx.progress(), "Warning [%s]: alter workflow %s: '%s' has %d boundary events; "+
			"`drop %s boundary event` drops the first. %s\n",
			boundaryDropAmbiguous.Code, s.Name, o.ActivityRef, n, o.ActivityRef,
			boundaryDropAmbiguous.Warning(ctx.LanguageVersion))
	}
	return nil
}

// storedBoundaryEvents returns an activity's boundary events, or nil for an
// activity kind that holds none (or no activity).
func storedBoundaryEvents(a workflows.WorkflowActivity) []*workflows.BoundaryEvent {
	switch t := a.(type) {
	case *workflows.UserTask:
		return t.BoundaryEvents
	case *workflows.CallMicroflowTask:
		return t.BoundaryEvents
	case *workflows.CallWorkflowActivity:
		return t.BoundaryEvents
	case *workflows.WaitForNotificationActivity:
		return t.BoundaryEvents
	}
	return nil
}
