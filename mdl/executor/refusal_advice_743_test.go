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

// A plain `create` of an existing workflow or OData client is refused, and the
// refusal used to say "use create or modify". For these two types that is the
// rewrite that loses Studio Pro-authored content (#743: event sub-processes and
// outcome flows of a workflow; UseQuerySegment, the catalog, proxy and microflow
// settings and the icon of an OData client). Until the carry lands the advice
// points to `alter` and says why.

func assertSafeAdvice(t *testing.T, err error, alter string) {
	t.Helper()
	if err == nil {
		t.Fatal("a plain create of an existing document was not refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, alter) {
		t.Errorf("the refusal does not point to %q: %s", alter, msg)
	}
	if strings.Contains(msg, "use create or modify to") {
		t.Errorf("the refusal still recommends the lossy rewrite: %s", msg)
	}
}

func TestCreateODataClient_ExistsAdvisesAlter(t *testing.T) {
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
	prog := parseMDL(t, "create odata client MyModule.Api (\n  ODataVersion: OData4,\n  MetadataUrl: 'https://example.com/odata/$metadata'\n);")
	assertSafeAdvice(t, createODataClient(ctx, prog.Statements[0].(*ast.CreateODataClientStmt)), "alter odata client")
}

func TestCreateWorkflow_ExistsAdvisesAlter(t *testing.T) {
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
	assertSafeAdvice(t, execCreateWorkflow(ctx, prog.Statements[0].(*ast.CreateWorkflowStmt)), "alter workflow")
}
