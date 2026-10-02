// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// grantWriteRightsFixture builds one entity carrying the three attribute shapes
// that matter to CE6592 — a plain one, a calculated one, and an autonumber — and
// captures the access rule a GRANT writes for it.
type grantWriteRightsFixture struct {
	ctx      *ExecContext
	captured *backend.EntityAccessRuleParams
}

func newGrantWriteRightsFixture(t *testing.T) *grantWriteRightsFixture {
	t.Helper()

	mod := mkModule("FieldService")
	h := mkHierarchy(mod)

	req := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: model.ID("e-req")},
		Name:        "ServiceRequest",
		Attributes: []*domainmodel.Attribute{
			{Name: "Description", Type: &domainmodel.StringAttributeType{Length: 200}},
			{
				Name:  "TotalCost",
				Type:  &domainmodel.DecimalAttributeType{},
				Value: &domainmodel.AttributeValue{Type: "CalculatedValue", MicroflowName: "FieldService.CalcTotal"},
			},
			{Name: "RequestNumber", Type: &domainmodel.AutoNumberAttributeType{}},
		},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-fs"},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{req},
	}

	f := &grantWriteRightsFixture{}
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dm}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "Coordinator"}}}, nil
		},
		AddEntityAccessRuleFunc: func(p backend.EntityAccessRuleParams) error {
			cp := p
			f.captured = &cp
			return nil
		},
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	f.ctx = ctx
	return f
}

func (f *grantWriteRightsFixture) attrRights(ref string) (string, bool) {
	if f.captured == nil {
		return "", false
	}
	for _, ma := range f.captured.MemberAccesses {
		if ma.AttributeRef == ref {
			return ma.AccessRights, true
		}
	}
	return "", false
}

// TestGrantWriteAll_AutoNumberDowngradedToReadOnly pins ako/mxcli#524: `grant
// write *` on an entity with an autonumber wrote ReadWrite on it and the build
// failed CE6592, so the user had to narrow the grant with a hand-written REVOKE.
//
// The calculated attribute in the same fixture is the CONTROL. It was already
// downgraded before this fix, so a test asserting only the autonumber could pass
// against a build where the predicate had simply been widened to "every
// attribute" — which would silently strip write rights from the whole model.
// Asserting the plain attribute keeps ReadWrite is what makes the pair mean
// something.
func TestGrantWriteAll_AutoNumberDowngradedToReadOnly(t *testing.T) {
	f := newGrantWriteRightsFixture(t)

	stmt := &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "FieldService", Name: "ServiceRequest"},
		Roles:  []ast.QualifiedName{{Module: "FieldService", Name: "Coordinator"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessWriteAll}},
	}
	if err := execGrantEntityAccess(f.ctx, stmt); err != nil {
		t.Fatalf("grant failed: %v", err)
	}

	const autoRef = "FieldService.ServiceRequest.RequestNumber"
	got, ok := f.attrRights(autoRef)
	if !ok {
		t.Fatalf("no MemberAccess for the autonumber at all; got %+v", f.captured.MemberAccesses)
	}
	if got != "ReadOnly" {
		t.Errorf("autonumber rights = %q, want ReadOnly — write on an autonumber is CE6592", got)
	}

	// Control 1: the calculated attribute, already covered before the fix.
	if got, _ := f.attrRights("FieldService.ServiceRequest.TotalCost"); got != "ReadOnly" {
		t.Errorf("calculated rights = %q, want ReadOnly", got)
	}
	// Control 2: an ordinary attribute must keep the write the statement asked
	// for. Without this the test passes against a blanket downgrade.
	if got, _ := f.attrRights("FieldService.ServiceRequest.Description"); got != "ReadWrite" {
		t.Errorf("plain attribute rights = %q, want ReadWrite — the grant asked for write", got)
	}
}

// TestGrantWriteAll_SystemReadOnlyAttributeAtTheWrite pins the GRANT's own
// rule, before any reconcile runs: `grant write *` on a System.FileDocument
// specialization must write HasContents ReadOnly (CE6592 otherwise, measured on
// mxbuild 11.14.0), and keep the inherited Name — measured clean with write —
// at ReadWrite. The PedApp test covers the stored result; this one the writer.
func TestGrantWriteAll_SystemReadOnlyAttributeAtTheWrite(t *testing.T) {
	mod := mkModule("Docs")
	sys := mkModule("System")
	doc := &domainmodel.Entity{
		BaseElement:       model.BaseElement{ID: model.ID("e-doc")},
		Name:              "Doc",
		GeneralizationRef: "System.FileDocument",
		Attributes:        []*domainmodel.Attribute{{Name: "Title", Type: &domainmodel.StringAttributeType{Length: 100}}},
	}
	fileDoc := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: model.ID("System.FileDocument")},
		Name:        "FileDocument",
		Attributes: []*domainmodel.Attribute{
			{Name: "Name", Type: &domainmodel.StringAttributeType{Length: 400}},
			{Name: "HasContents", Type: &domainmodel.BooleanAttributeType{}},
		},
	}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-docs"}, ContainerID: mod.ID, Entities: []*domainmodel.Entity{doc}}
	sysDm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-sys"}, ContainerID: sys.ID, Entities: []*domainmodel.Entity{fileDoc}}

	var captured *backend.EntityAccessRuleParams
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod, sys}, nil },
		GetModuleByNameFunc: func(n string) (*model.Module, error) {
			if n == "System" {
				return sys, nil
			}
			return mod, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm, sysDm}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == sys.ID {
				return sysDm, nil
			}
			return dm, nil
		},
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
		AddEntityAccessRuleFunc: func(p backend.EntityAccessRuleParams) error {
			cp := p
			captured = &cp
			return nil
		},
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	stmt := &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "Docs", Name: "Doc"},
		Roles:  []ast.QualifiedName{{Module: "Docs", Name: "User"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessReadAll}, {Type: ast.EntityAccessWriteAll}},
	}
	if err := execGrantEntityAccess(ctx, stmt); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	if captured == nil {
		t.Fatal("no access rule written")
	}
	rights := map[string]string{}
	for _, ma := range captured.MemberAccesses {
		rights[ma.AttributeRef] = ma.AccessRights
	}
	if got := rights["System.FileDocument.HasContents"]; got != "ReadOnly" {
		t.Errorf("HasContents = %q, want ReadOnly (CE6592); all: %v", got, rights)
	}
	if got := rights["System.FileDocument.Name"]; got != "ReadWrite" {
		t.Errorf("control: inherited Name = %q, want ReadWrite; all: %v", got, rights)
	}
}
