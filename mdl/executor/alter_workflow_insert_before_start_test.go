// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// `insert before` is new with the generic alter (ako/mxcli#712), and a flow's
// start activity is an ordinary, addressable member of it (`start1`). Inserting
// before it wrote a flow that no longer begins with its start event: exec
// reported "Altered workflow" and mx check (11.14.0, TestApp) answered CE9526
// "Main process in workflow should start with a start event."
func TestAlterWorkflow_InsertBeforeStartIsRefused(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "WF")
	wf.Flow = &workflows.Flow{
		Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "s1"}, Name: "start1", Caption: "Start"}},
			&workflows.UserTask{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "a1"}, Name: "task1"}},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				BaseElement: model.BaseElement{ID: "e1"}, Name: "end1", Caption: "End"}},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	for _, src := range []string{
		"alter workflow Sales.WF { insert before start1 { wait for notification Ready; } };",
		"alter workflow Sales.WF { insert before 'Start' { wait for notification Ready; } };",
	} {
		errs := alterWfRefErrors(t, ctx, src)
		if len(errs) != 1 || !strings.Contains(errs[0], "start") || !strings.Contains(errs[0], "insert after") {
			t.Errorf("%s: want one refusal pointing at `insert after`, got %q", src, errs)
		}
	}

	// Control: the same fragment before an ordinary activity is accepted.
	if errs := alterWfRefErrors(t, ctx, "alter workflow Sales.WF { insert before task1 { wait for notification Ready; } };"); len(errs) != 0 {
		t.Errorf("insert before task1: want no refusal, got %q", errs)
	}
}
