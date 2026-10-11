// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
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

// ledgerResyncFixture is module Ledger with Transaction (TxDate localized,
// CreatedAt localized, Memo String(100)) and three views over it:
//   - VDaySpend reads t.TxDate, stored LocalizeDate true (in sync today);
//   - VCreated reads t.CreatedAt, stored LocalizeDate false — out of sync on
//     purpose, so a resync that touched views not reading the changed attribute
//     would rewrite it (the control);
//   - VMemo reads t.Memo as String(100).
//
// UpdateEntity replaces the entity in the domain model, as the real backend's
// next read would, and records every write by entity name.
type ledgerResyncFixture struct {
	ctx     *ExecContext
	out     *bytes.Buffer
	dm      *domainmodel.DomainModel
	written []string
}

func newLedgerResyncFixture(t *testing.T) *ledgerResyncFixture {
	t.Helper()
	mod := mkModule("Ledger")
	tx := mkEntity(mod.ID, "Transaction")
	tx.Attributes = []*domainmodel.Attribute{
		{BaseElement: model.BaseElement{ID: "attr-txdate"}, Name: "TxDate", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}},
		{BaseElement: model.BaseElement{ID: "attr-created"}, Name: "CreatedAt", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}},
		{BaseElement: model.BaseElement{ID: "attr-memo"}, Name: "Memo", Type: &domainmodel.StringAttributeType{Length: 100}},
	}
	view := func(name, oql string, attrs ...*domainmodel.Attribute) *domainmodel.Entity {
		v := mkEntity(mod.ID, name)
		v.Source = "DomainModels$OqlViewEntitySource"
		v.SourceDocumentRef = "Ledger." + name
		v.OqlQuery = oql
		v.Attributes = attrs
		return v
	}
	daySpend := view("VDaySpend", "select t.TxDate as TxDate from Ledger.Transaction as t",
		&domainmodel.Attribute{Name: "TxDate", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}})
	created := view("VCreated", "select t.CreatedAt as CreatedAt from Ledger.Transaction as t",
		&domainmodel.Attribute{Name: "CreatedAt", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: false}})
	memo := view("VMemo", "select t.Memo as Memo from Ledger.Transaction as t",
		&domainmodel.Attribute{Name: "Memo", Type: &domainmodel.StringAttributeType{Length: 100}})
	f := &ledgerResyncFixture{dm: mkDomainModel(mod.ID, tx, daySpend, created, memo)}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{f.dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return f.dm, nil },
		UpdateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			f.written = append(f.written, e.Name)
			for i, old := range f.dm.Entities {
				if old.ID == e.ID {
					f.dm.Entities[i] = e
				}
			}
			return nil
		},
	}
	f.ctx, f.out = newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return f
}

func (f *ledgerResyncFixture) exec(t *testing.T, src string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	for _, stmt := range prog.Statements {
		var err error
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			err = execCreateEntity(f.ctx, s)
		case *ast.AlterEntityStmt:
			err = execAlterEntity(f.ctx, s)
		default:
			t.Fatalf("unexpected statement %T", stmt)
		}
		if err != nil {
			t.Fatalf("exec %q: %v", src, err)
		}
	}
}

func (f *ledgerResyncFixture) entity(name string) *domainmodel.Entity {
	for _, e := range f.dm.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (f *ledgerResyncFixture) wrote(name string) int {
	n := 0
	for _, w := range f.written {
		if w == name {
			n++
		}
	}
	return n
}

// ako/mxcli-ledger FINDINGS #169: flipping a source DateTime to `not
// localized` left every view selecting it with LocalizeDate true, and mx check
// reported CE6770 "View Entity is out of sync with the OQL Query" per view —
// check and exec said nothing. The view column is re-derived from the source
// after the source is written; a second run writes no view; a view that does
// not read the changed attribute is never written.
func TestSourceLocalizeChange_ResyncsViews(t *testing.T) {
	stmts := map[string]string{
		"alter modify attribute": `alter entity Ledger.Transaction modify attribute TxDate: DateTime not localized;`,
		"create or modify": `create or modify persistent entity Ledger.Transaction (
  TxDate: DateTime not localized,
  CreatedAt: DateTime,
  Memo: String(100)
);`,
	}
	for name, src := range stmts {
		t.Run(name, func(t *testing.T) {
			f := newLedgerResyncFixture(t)
			f.exec(t, src)

			dt := f.entity("VDaySpend").Attributes[0].Type.(*domainmodel.DateTimeAttributeType)
			if dt.LocalizeDate {
				t.Errorf("VDaySpend.TxDate LocalizeDate = true over a non-localized source — "+
					"mxbuild reports CE6770 \"View Entity is out of sync with the OQL Query\"\noutput:\n%s", f.out)
			}
			if f.wrote("VDaySpend") != 1 {
				t.Errorf("VDaySpend written %d times, want 1", f.wrote("VDaySpend"))
			}
			if !strings.Contains(f.out.String(), "Ledger.VDaySpend (TxDate: not localized)") {
				t.Errorf("the resync was not reported:\n%s", f.out)
			}
			// Control: VCreated is out of sync too, but reads nothing that changed.
			if n := f.wrote("VCreated"); n != 0 {
				t.Errorf("VCreated, which does not read TxDate, was written %d time(s)", n)
			}
			if n := f.wrote("VMemo"); n != 0 {
				t.Errorf("VMemo, which does not read TxDate, was written %d time(s)", n)
			}

			// A second run: the source no longer changes, so no view is written.
			f.written = nil
			f.out.Reset()
			f.exec(t, src)
			if n := f.wrote("VDaySpend"); n != 0 {
				t.Errorf("repeat run rewrote VDaySpend %d time(s); want none\noutput:\n%s", n, f.out)
			}
		})
	}
}

// A view column type that is declared in the view text (here a String length)
// cannot be re-derived, so a source change that breaks it is a warning naming
// the view and column; the view is not written. Exec-only: `check` does not
// apply the source change, so it has no changed attribute to look from.
func TestSourceLengthChange_WarnsStaleView(t *testing.T) {
	f := newLedgerResyncFixture(t)
	f.exec(t, `alter entity Ledger.Transaction modify attribute Memo: String(200);`)
	out := f.out.String()
	if !strings.Contains(out, "Ledger.VMemo") || !strings.Contains(out, "Memo declared String(100)") {
		t.Errorf("no stale-view warning naming Ledger.VMemo and its Memo column:\n%s", out)
	}
	if strings.Contains(out, "Ledger.VDaySpend") || strings.Contains(out, "Ledger.VCreated") {
		t.Errorf("a view not reading Memo was reported:\n%s", out)
	}
	if n := f.wrote("VMemo"); n != 0 {
		t.Errorf("VMemo was written %d time(s); a declared type is the view's to change", n)
	}
}
