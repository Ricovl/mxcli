// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestShowMicroflows_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "ACT_CreateOrder")

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listMicroflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.ACT_CreateOrder")
	assertContainsStr(t, out, "(1 microflows)")
}

func TestShowMicroflows_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	mf1 := mkMicroflow(mod1.ID, "ACT_Sell")
	mf2 := mkMicroflow(mod2.ID, "ACT_Hire")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, mf1.ContainerID, mod1.ID)
	withContainer(h, mf2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf1, mf2}, nil },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listMicroflows(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales.ACT_Sell")
	assertContainsStr(t, out, "HR.ACT_Hire")
}

func TestShowNanoflows_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	nf := mkNanoflow(mod.ID, "NF_Validate")

	h := mkHierarchy(mod)
	withContainer(h, nf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listNanoflows(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.NF_Validate")
	assertContainsStr(t, out, "(1 nanoflows)")
}

func TestDescribeMicroflow_Mock_Minimal(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "ACT_DoSomething")

	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "ACT_DoSomething"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify microflow MyModule.ACT_DoSomething")
}

func TestDescribeMicroflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

func TestDescribeMicroflow_UsesDirectQualifiedNameLookup(t *testing.T) {
	mod := mkModule("MyModule")
	mf := mkMicroflow(mod.ID, "Direct")
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)

	directCalls := 0
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetMicroflowByNameFunc: func(name string) (*microflows.Microflow, error) {
			directCalls++
			if name != "MyModule.Direct" {
				t.Fatalf("direct lookup name = %q", name)
			}
			return mf, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			t.Fatal("DESCRIBE must not list every microflow")
			return nil, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeMicroflow(ctx, ast.QualifiedName{Module: "MyModule", Name: "Direct"}))
	if directCalls != 1 {
		t.Fatalf("GetMicroflowByName called %d times, want 1", directCalls)
	}
}

func TestBuildEntityEnumAttrMap_CachesLightweightMetadata(t *testing.T) {
	mod := mkModule("Sales")
	dm := &domainmodel.DomainModel{
		ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{{
			Name: "Order",
			Attributes: []*domainmodel.Attribute{{
				Name: "Status",
				Type: &domainmodel.EnumerationAttributeType{EnumerationRef: "Sales.OrderStatus"},
			}},
		}},
	}
	calls := 0
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			calls++
			return []*domainmodel.DomainModel{dm}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	for range 3 {
		attrs := buildEntityEnumAttrMap(ctx, "Sales.Order")
		if attrs["Status"] != "Sales.OrderStatus" {
			t.Fatalf("enum metadata = %#v", attrs)
		}
	}
	if calls != 1 {
		t.Fatalf("ListDomainModels called %d times, want 1", calls)
	}

	invalidateDomainModelsCache(ctx)
	buildEntityEnumAttrMap(ctx, "Sales.Order")
	if calls != 2 {
		t.Fatalf("ListDomainModels called %d times after invalidation, want 2", calls)
	}
}

// Backend error: cmd_error_mock_test.go (TestShowMicroflows_Mock_BackendError, TestShowNanoflows_Mock_BackendError)
// JSON: cmd_json_mock_test.go (TestShowMicroflows_Mock_JSON, TestShowNanoflows_Mock_JSON)

// --- OBS-2: Module not found error for SHOW MICROFLOWS ---

func TestShowMicroflows_Mock_ModuleNotFound(t *testing.T) {
	mod := mkModule("Sales")

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	err := listMicroflows(ctx, "NonExistent")
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// --- OBS-8: Empty microflow name validation ---

func TestCreateMicroflow_Mock_EmptyName(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb))
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: ""},
	}
	err := execCreateMicroflow(ctx, stmt)
	assertError(t, err)
	assertContainsStr(t, err.Error(), "must not be empty")
}
