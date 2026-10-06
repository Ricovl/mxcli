// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// localizeFixture is a module with one persistent entity, Sale, carrying a
// non-localized DateTime (SaleDate — Studio Pro's "Localize" unticked, the
// normal choice for a calendar date) and a localized one (CreatedAt).
func localizeFixture(t *testing.T) (*ExecContext, *[]*domainmodel.Entity) {
	t.Helper()
	mod := mkModule("Shop")
	sale := mkEntity(mod.ID, "Sale")
	sale.Attributes = []*domainmodel.Attribute{
		{Name: "SaleDate", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: false}},
		{Name: "CreatedAt", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}},
		{Name: "Amount", Type: &domainmodel.IntegerAttributeType{}},
	}
	dm := mkDomainModel(mod.ID, sale)
	var created []*domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = append(created, e)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, &created
}

func parseViewStmt(t *testing.T, src string) *ast.CreateViewEntityStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateViewEntityStmt)
	if !ok {
		t.Fatalf("not a CREATE VIEW ENTITY: %T", prog.Statements[0])
	}
	return s
}

// mendixlabs/mxcli#1297: a view attribute over a NON-localized DateTime column
// was always written LocalizeDate = true, and mx check reported CE6770 "View
// Entity is out of sync with the OQL Query". The flag is derived from the
// source attribute the column reads. Each shape below was measured on mxbuild
// 11.12.5: written localized, every one is CE6770; written non-localized, 0
// errors.
func TestExecCreateViewEntity_DateTimeInheritsSourceLocalizeDate(t *testing.T) {
	cases := []struct {
		name, column string
		want         bool
	}{
		{"pass-through, the reported case", "s.SaleDate as D", false},
		{"slash path", "s/SaleDate as D", false},
		{"max", "max(s.SaleDate) as D", false},
		{"min", "MIN(s.SaleDate) AS D", false},
		{"case over the column", "case when s.Amount > 0 then s.SaleDate else s.SaleDate end as D", false},
		{"localized source stays localized", "s.CreatedAt as D", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, created := localizeFixture(t)
			s := parseViewStmt(t, `create view entity Shop.SalePerDay (
  D: DateTime
) as (
  select `+c.column+`
  from Shop.Sale as s
);`)
			if err := execCreateViewEntity(ctx, s); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if len(*created) != 1 {
				t.Fatalf("want one created entity, got %d", len(*created))
			}
			dt, ok := (*created)[0].Attributes[0].Type.(*domainmodel.DateTimeAttributeType)
			if !ok {
				t.Fatalf("D is %T, want DateTime", (*created)[0].Attributes[0].Type)
			}
			if dt.LocalizeDate != c.want {
				t.Errorf("SalePerDay.D LocalizeDate = %v, want %v — mxbuild reports CE6770 "+
					"\"View Entity is out of sync with the OQL Query\"", dt.LocalizeDate, c.want)
			}
		})
	}
}

// A DateTime column with no source attribute behind it keeps Mendix's default.
func TestViewDateTimeLocalize_NoSourceKeepsDefault(t *testing.T) {
	ctx, _ := localizeFixture(t)
	attrs := []ast.ViewAttribute{{Name: "D", Type: ast.DataType{Kind: ast.TypeDateTime}}}
	got := viewDateTimeLocalize(ctx, "select '[%CurrentDateTime%]' as D from Shop.Sale as s", attrs)
	if _, ok := got["D"]; ok {
		t.Errorf("a column with no source attribute was given a LocalizeDate: %v", got)
	}
	// Two sources that disagree: unmeasured, so no guess.
	got = viewDateTimeLocalize(ctx,
		"select coalesce(s.SaleDate, s.CreatedAt) as D from Shop.Sale as s", attrs)
	if _, ok := got["D"]; ok {
		t.Errorf("sources that disagree were resolved to %v; want no derivation", got)
	}
}
