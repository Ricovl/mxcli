// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1525: a business event message attribute named like a member
// the event entity inherits from BusinessEvents.PublishedBusinessEvent (the
// reporter's `EventId`) passed check and exec, and mxbuild reported the
// duplicate. The inherited members are read from the project, never from a list
// in mxcli: the marketplace module's entity is what decides.

// beFixture: BusinessEvents.PublishedBusinessEvent (EventId, Subject) — the
// names stand in for whatever the installed module declares — and
// Shop.PBE_Order (OrderId) extending it. withBusinessEvents=false leaves the
// BusinessEvents module out of the project.
func beFixture(t *testing.T, withBusinessEvents bool) *ExecContext {
	t.Helper()
	be := &model.Module{Name: "BusinessEvents"}
	be.ID = nextID("bemod")
	shop := &model.Module{Name: "Shop"}
	shop.ID = nextID("shop")

	mkAttr := func(name string) *domainmodel.Attribute {
		a := &domainmodel.Attribute{Name: name, Type: &domainmodel.StringAttributeType{}}
		a.ID = nextID("attr" + name)
		return a
	}
	pbe := &domainmodel.Entity{Name: "PublishedBusinessEvent", Persistable: true}
	pbe.ID = nextID("pbe")
	pbe.Attributes = []*domainmodel.Attribute{mkAttr("EventId"), mkAttr("Subject")}
	order := &domainmodel.Entity{Name: "PBE_Order", Persistable: true, GeneralizationRef: "BusinessEvents.PublishedBusinessEvent"}
	order.ID = nextID("order")
	order.Attributes = []*domainmodel.Attribute{mkAttr("OrderId")}

	beDM := &domainmodel.DomainModel{ContainerID: be.ID, Entities: []*domainmodel.Entity{pbe}}
	beDM.ID = nextID("bedm")
	shopDM := &domainmodel.DomainModel{ContainerID: shop.ID, Entities: []*domainmodel.Entity{order}}
	shopDM.ID = nextID("shopdm")

	modules := []*model.Module{shop}
	if withBusinessEvents {
		modules = append(modules, be)
	}
	h := mkHierarchy(modules...)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return modules, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			for _, m := range modules {
				if m.Name == name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("no module %q", name)
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch {
			case id == shop.ID:
				return shopDM, nil
			case id == be.ID && withBusinessEvents:
				return beDM, nil
			}
			return nil, fmt.Errorf("no domain model %s", id)
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func beServiceStmt(entity string, attrs ...string) *ast.CreateBusinessEventServiceStmt {
	msg := &ast.BusinessEventMessageDef{MessageName: "OrderCreated", Operation: "publish", Entity: entity}
	for _, a := range attrs {
		msg.Attributes = append(msg.Attributes, &ast.BusinessEventAttributeDef{Name: a, TypeName: "Long"})
	}
	return &ast.CreateBusinessEventServiceStmt{
		Name:     ast.QualifiedName{Module: "Shop", Name: "OrderEvents"},
		Messages: []*ast.BusinessEventMessageDef{msg},
	}
}

func runBEMemberCheck(ctx *ExecContext, stmts ...ast.Statement) []error {
	prog := &ast.Program{Statements: stmts}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateBusinessEventInheritedMembers(ctx, prog, sc)
}

func TestValidateBusinessEventMembers_InheritedNameOnProjectEntity(t *testing.T) {
	errs := runBEMemberCheck(beFixture(t, true), beServiceStmt("Shop.PBE_Order", "OrderId", "EventId"))
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	for _, want := range []string{"OrderCreated", "EventId", "BusinessEvents.PublishedBusinessEvent", "Shop.PBE_Order"} {
		if !strings.Contains(errs[0].Error(), want) {
			t.Errorf("error should mention %q: %v", want, errs[0])
		}
	}
}

// The usual script creates the event entity next to the service: the chain then
// starts in the script and continues in the project. Case differs on purpose —
// two members differing only in case are one member to Mendix.
func TestValidateBusinessEventMembers_InheritedNameOnScriptEntity(t *testing.T) {
	ent := &ast.CreateEntityStmt{
		Name:           ast.QualifiedName{Module: "Shop", Name: "PBE_New"},
		Generalization: &ast.QualifiedName{Module: "BusinessEvents", Name: "PublishedBusinessEvent"},
		Attributes:     []ast.Attribute{{Name: "OrderId"}},
	}
	errs := runBEMemberCheck(beFixture(t, true), ent, beServiceStmt("Shop.PBE_New", "OrderId", "eventid"))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "eventid") {
		t.Fatalf("want one error naming eventid, got %v", errs)
	}
}

// CONTROL: the entity's own attributes are exactly what a message must declare,
// so they never collide; neither does anything when the project cannot say what
// is inherited (no BusinessEvents module) or the message names no entity.
func TestValidateBusinessEventMembers_SilentWhenNothingIsInherited(t *testing.T) {
	cases := []struct {
		name string
		ctx  *ExecContext
		stmt ast.Statement
	}{
		{"own attributes only", beFixture(t, true), beServiceStmt("Shop.PBE_Order", "OrderId")},
		{"module absent", beFixture(t, false), beServiceStmt("Shop.PBE_Order", "OrderId", "EventId")},
		{"no entity", beFixture(t, true), beServiceStmt("", "EventId")},
	}
	for _, c := range cases {
		if errs := runBEMemberCheck(c.ctx, c.stmt); len(errs) != 0 {
			t.Errorf("%s: want no error, got %v", c.name, errs)
		}
	}
}

// The check is wired into the --references pass.
func TestValidateBusinessEventMembers_RunsInReferencePass(t *testing.T) {
	errs, _ := validateProgramWithWarnings(beFixture(t, true),
		&ast.Program{Statements: []ast.Statement{beServiceStmt("Shop.PBE_Order", "OrderId", "EventId")}})
	for _, e := range errs {
		if strings.Contains(e.Error(), "EventId") && strings.Contains(e.Error(), "inherit") {
			return
		}
	}
	t.Fatalf("reference pass did not report the inherited EventId: %v", errs)
}
