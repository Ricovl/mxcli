// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// endBoundaryPathsLikeStudioPro ends the interrupting boundary paths of a
// workflow built from CREATE the way Studio Pro does — see
// workflows.EndInterruptingBoundaryPaths (ako/mxcli#1712). The main flow and
// each event sub-process body may hold an End (an End closing an interrupting
// path in a sub-process built clean on 11.15.0).
func endBoundaryPathsLikeStudioPro(main []workflows.WorkflowActivity, esps []*workflows.EventSubProcess) {
	workflows.EndInterruptingBoundaryPaths(main, true, newWorkflowID)
	for _, esp := range esps {
		if esp != nil && esp.Flow != nil {
			workflows.EndInterruptingBoundaryPaths(esp.Flow.Activities, true, newWorkflowID)
		}
	}
}

// alterEndPlacement answers, for ALTER WORKFLOW, whether an End may close an
// interrupting boundary path placed at an activity of the stored workflow. An
// activity the stored workflow does not show unambiguously — one the statement
// inserts, one in an event sub-process — gets no: its path keeps the marker,
// which builds everywhere.
type alterEndPlacement struct {
	wf *workflows.Workflow
}

func newAlterEndPlacement(ctx *ExecContext, s *ast.AlterWorkflowStmt) *alterEndPlacement {
	return &alterEndPlacement{wf: findStoredWorkflow(ctx, s.Name)}
}

func (p *alterEndPlacement) endAllowedAt(ref string, atPos int) bool {
	if p == nil || p.wf == nil || p.wf.Flow == nil {
		return false
	}
	place, ok := resolveStoredPlacement(p.wf.Flow, ref, atPos)
	return ok && !place.underSplit && !place.underNonInterrupting
}

// endInserted ends the interrupting paths nested in activities an ALTER inserts
// at ref.
func (p *alterEndPlacement) endInserted(acts []workflows.WorkflowActivity, ref string, atPos int) {
	workflows.EndInterruptingBoundaryPaths(acts, p.endAllowedAt(ref, atPos), newWorkflowID)
}

// endInsertedBoundaryPath ends the path of a boundary event an ALTER inserts on
// ref: an End for an interrupting event where one may stand — the mutator then
// appends no marker — and the same rule for paths nested in it.
func (p *alterEndPlacement) endInsertedBoundaryPath(acts []workflows.WorkflowActivity, eventType, ref string, atPos int) []workflows.WorkflowActivity {
	allowed := p.endAllowedAt(ref, atPos)
	interrupting := strings.HasPrefix(eventType, "Interrupting")
	workflows.EndInterruptingBoundaryPaths(acts, allowed && interrupting, newWorkflowID)
	if allowed && interrupting {
		return workflows.EndInterruptingPath(acts, newWorkflowID)
	}
	return acts
}
