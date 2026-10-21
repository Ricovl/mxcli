// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1498 (follow-up, point 2): describe prints an entity's,
// attribute's, association's and enumeration's `export level api`, and the
// printed statement parses back to the same level — so describe -> exec keeps a
// Usable element Usable. A Hidden one prints no clause (the default), which is
// the control: a describe that printed the clause for every element would pass
// the API half alone.
func TestDescribe_DomainModelExportLevelRoundTrips(t *testing.T) {
	mod := mkModule("M")
	e := mkEntity(mod.ID, "E")
	e.ExportLevel = "API"
	e.Attributes = []*domainmodel.Attribute{
		{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: "Usable", Type: &domainmodel.StringAttributeType{Length: 10}, ExportLevel: "API"},
		{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: "Plain", Type: &domainmodel.StringAttributeType{Length: 10}, ExportLevel: "Hidden"},
	}
	f := mkEntity(mod.ID, "F")
	f.ExportLevel = "Hidden"
	dm := mkDomainModel(mod.ID, e, f)
	usable := mkAssociation(mod.ID, "E_F", e.ID, f.ID)
	usable.ExportLevel = "API"
	plain := mkAssociation(mod.ID, "F_E", f.ID, e.ID)
	plain.ExportLevel = "Hidden"
	dm.Associations = []*domainmodel.Association{usable, plain}
	dm.CrossAssociations = []*domainmodel.CrossModuleAssociation{{
		BaseElement: model.BaseElement{ID: nextID("cross")}, Name: "E_User", ParentID: e.ID, ChildRef: "System.User",
		Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault, ExportLevel: "API",
	}}
	kind := mkEnumeration(mod.ID, "Kind", "A")
	kind.ExportLevel = "API"
	other := mkEnumeration(mod.ID, "Other", "A")
	other.ExportLevel = "Hidden"

	mods := []*model.Module{mod}
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return mods, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{kind, other}, nil },
	}), withHierarchy(mkHierarchy(mods...)))
	buf := ctx.Output.(*bytes.Buffer)
	describe := func(f func() error) *ast.Program {
		t.Helper()
		buf.Reset()
		if err := f(); err != nil {
			t.Fatal(err)
		}
		return parseMDL(t, buf.String())
	}
	qn := func(name string) ast.QualifiedName { return ast.QualifiedName{Module: "M", Name: name} }
	level := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}

	ent := firstStmt[*ast.CreateEntityStmt](t, describe(func() error { return describeEntity(ctx, qn("E")) }))
	if level(ent.ExportLevel) != "API" {
		t.Errorf("describe entity M.E re-parses with ExportLevel %q, want API", level(ent.ExportLevel))
	}
	for _, a := range ent.Attributes {
		want := map[string]string{"Usable": "API", "Plain": ""}[a.Name]
		if level(a.ExportLevel) != want {
			t.Errorf("attribute %s re-parses with ExportLevel %q, want %q", a.Name, level(a.ExportLevel), want)
		}
	}
	if ent := firstStmt[*ast.CreateEntityStmt](t, describe(func() error { return describeEntity(ctx, qn("F")) })); ent.ExportLevel != nil {
		t.Errorf("describe of a Hidden entity printed export level %q", *ent.ExportLevel)
	}

	for name, want := range map[string]string{"E_F": "API", "F_E": "", "E_User": "API"} {
		a := firstStmt[*ast.CreateAssociationStmt](t, describe(func() error { return describeAssociation(ctx, qn(name)) }))
		if level(a.ExportLevel) != want {
			t.Errorf("describe association M.%s re-parses with ExportLevel %q, want %q", name, level(a.ExportLevel), want)
		}
	}

	for name, want := range map[string]string{"Kind": "API", "Other": ""} {
		en := firstStmt[*ast.CreateEnumerationStmt](t, describe(func() error { return describeEnumeration(ctx, qn(name)) }))
		if level(en.ExportLevel) != want {
			t.Errorf("describe enumeration M.%s re-parses with ExportLevel %q, want %q", name, level(en.ExportLevel), want)
		}
	}
	if strings.Contains(buf.String(), "Studio Pro") {
		t.Errorf("describe still points at Studio Pro for the level:\n%s", buf.String())
	}
}

func firstStmt[T ast.Statement](t *testing.T, prog *ast.Program) T {
	t.Helper()
	for _, s := range prog.Statements {
		if v, ok := s.(T); ok {
			return v
		}
	}
	var zero T
	t.Fatalf("no %T in %d statements", zero, len(prog.Statements))
	return zero
}
