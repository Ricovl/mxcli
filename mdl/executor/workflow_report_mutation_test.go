// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Re-running `create or modify workflow` printed "Created workflow: …" on every
// run, for a rewrite that changed nothing and whose unit write was elided. Every
// other document type reports through ReportMutation and says "Unchanged".
func TestCreateOrModifyWorkflow_ReportsWhatHappened(t *testing.T) {
	for _, tc := range []struct {
		name    string
		written int
		want    string
	}{
		{"elided rewrite", 0, "Unchanged workflow: MyModule.Approve"},
		{"rewrite that landed (control)", 1, "Modified workflow: MyModule.Approve"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			stored := storedNamedWorkflow(mod)
			h := mkHierarchy(mod)
			withContainer(h, stored.ContainerID, mod.ID)
			cb := &countingBackend{}
			cb.MockBackend = &mock.MockBackend{
				IsConnectedFunc:   func() bool { return true },
				ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{stored}, nil },
				ListPagesFunc:     func() ([]*pages.Page, error) { return []*pages.Page{mkPage(mod.ID, "P")}, nil },
				UpdateWorkflowFunc: func(*workflows.Workflow) error {
					cb.offer(1, tc.written)
					return nil
				},
			}
			ctx, out := newMockCtx(t, withHierarchy(h))
			ctx.Backend = cb
			prog := parseMDL(t, namedWorkflowRewrite)
			assertNoError(t, execCreateWorkflow(ctx, prog.Statements[0].(*ast.CreateWorkflowStmt)))
			if got := out.String(); !strings.Contains(got, tc.want) {
				t.Fatalf("output %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
