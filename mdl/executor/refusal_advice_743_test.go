// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// A plain `create` of an existing workflow or OData client is refused. While
// `create or modify` lost Studio Pro-authored content for these two types (#743:
// event sub-processes, outcome flows and activity names of a workflow;
// UseQuerySegment, the catalog, proxy and microflow settings and the icon of an
// OData client) the refusal pointed to `alter` instead. The rewrite now carries
// them, so the refusal recommends it, as for every other type, and still names
// `alter` for a one-part change.

func assertCreateOrModifyAdvice(t *testing.T, err error, alter string) {
	t.Helper()
	if err == nil {
		t.Fatal("a plain create of an existing document was not refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "use create or modify to update") {
		t.Errorf("the refusal does not recommend create or modify: %s", msg)
	}
	if !strings.Contains(msg, alter) {
		t.Errorf("the refusal does not point to %q: %s", alter, msg)
	}
}

func TestCreateODataClient_ExistsAdvisesCreateOrModify(t *testing.T) {
	mod := mkModule("MyModule")
	existing := &model.ConsumedODataService{BaseElement: model.BaseElement{ID: nextID("cos")}, ContainerID: mod.ID, Name: "Api"}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{existing}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog := parseMDL(t, "create consumed odata service MyModule.Api (\n  ODataVersion: OData4,\n  MetadataUrl: 'https://example.com/odata/$metadata'\n);")
	assertCreateOrModifyAdvice(t, createODataClient(ctx, prog.Statements[0].(*ast.CreateODataClientStmt)), "alter consumed odata service")
}

func TestCreateWorkflow_ExistsAdvisesCreateOrModify(t *testing.T) {
	mod := mkModule("MyModule")
	existing := &workflows.Workflow{BaseElement: model.BaseElement{ID: nextID("wf")}, ContainerID: mod.ID, Name: "Approve"}
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{existing}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog := parseMDL(t, "create workflow MyModule.Approve\nbegin\nend workflow;")
	assertCreateOrModifyAdvice(t, execCreateWorkflow(ctx, prog.Statements[0].(*ast.CreateWorkflowStmt)), "alter workflow")
}
