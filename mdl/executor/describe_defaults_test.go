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
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A canonical DESCRIBE leaves out what the statement would default to anyway
// (R12, #748). Each case pins both halves of that: the default is not printed,
// and the text that IS printed reads back — through the parser — to the stored
// value, so omitting the clause cannot change what re-executing it writes.

// describeAssocText describes M.Child_Parent after shaping it with set.
func describeAssocText(t *testing.T, set func(a *domainmodel.Association)) (string, *ast.CreateAssociationStmt) {
	t.Helper()
	ctx, assoc := assocFixture(t)
	assoc.Type = domainmodel.AssociationTypeReference
	assoc.Owner = domainmodel.AssociationOwnerDefault
	assoc.StorageFormat = domainmodel.StorageFormatColumn
	assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences}
	if set != nil {
		set(assoc)
	}
	var buf bytes.Buffer
	ctx.Output = &buf
	assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))
	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n--- output ---\n%s", errs, buf.String())
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateAssociationStmt", prog.Statements[0])
	}
	return buf.String(), stmt
}

func TestDescribeAssociation_OmitsDefaults(t *testing.T) {
	got, stmt := describeAssocText(t, nil)
	want := "create or modify association M.Child_Parent\nfrom M.Child to M.Parent;\n"
	if got != want {
		t.Errorf("describe of an all-defaults association:\n got %q\nwant %q", got, want)
	}
	if stmt.Type != ast.AssocReference || stmt.Owner != ast.OwnerDefault ||
		stmt.Storage != ast.StorageDefault || stmt.DeleteBehavior != ast.DeleteKeepReferences {
		t.Errorf("the omitted clauses do not read back as the defaults: %+v", stmt)
	}
}

func TestDescribeAssociation_KeepsNonDefaults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(a *domainmodel.Association)
		line  string
		check func(s *ast.CreateAssociationStmt) bool
	}{
		{"reference set", func(a *domainmodel.Association) { a.Type = domainmodel.AssociationTypeReferenceSet },
			"type ReferenceSet", func(s *ast.CreateAssociationStmt) bool { return s.Type == ast.AssocReferenceSet }},
		{"owner both", func(a *domainmodel.Association) { a.Owner = domainmodel.AssociationOwnerBoth },
			"owner Both", func(s *ast.CreateAssociationStmt) bool { return s.Owner == ast.OwnerBoth }},
		// Table is NOT the create default: a fresh create stores Column, so a
		// table association must say so or re-executing it elsewhere changes
		// the database schema (#704).
		{"table storage", func(a *domainmodel.Association) { a.StorageFormat = domainmodel.StorageFormatTable },
			"storage table", func(s *ast.CreateAssociationStmt) bool { return s.Storage == ast.StorageTable }},
		{"cascade", func(a *domainmodel.Association) {
			a.ChildDeleteBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeAndReferences
		}, "on delete cascade", func(s *ast.CreateAssociationStmt) bool { return s.DeleteBehavior == ast.DeleteCascade }},
		// A message on the default behaviour is not a default.
		{"set null with message", func(a *domainmodel.Association) { a.ChildDeleteBehavior.ErrorMessage = "In use" },
			"on delete set null error message 'In use'",
			func(s *ast.CreateAssociationStmt) bool { return s.DeleteErrorMessage == "In use" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, stmt := describeAssocText(t, tc.set)
			if !strings.Contains(got, tc.line+";\n") {
				t.Errorf("want %q as the last clause:\n%s", tc.line, got)
			}
			if !tc.check(stmt) {
				t.Errorf("%q did not read back: %+v\n%s", tc.line, stmt, got)
			}
			for _, noise := range []string{"type Reference\n", "owner Default", "storage column", "on delete set null;"} {
				if strings.Contains(got, noise) {
					t.Errorf("default %q printed:\n%s", noise, got)
				}
			}
		})
	}
}

func TestFormatAction_LogOmitsDefaultLevelAndNode(t *testing.T) {
	msg := &model.Text{Translations: map[string]string{"en_US": "Hello"}}
	for _, tc := range []struct {
		level microflows.LogLevel
		node  string
		want  string
	}{
		{microflows.LogLevelInfo, "'Application'", "log 'Hello';"},
		{"", "", "log 'Hello';"},
		{microflows.LogLevelWarning, "'Application'", "log warning 'Hello';"},
		{microflows.LogLevelInfo, "'Orders'", "log node 'Orders' 'Hello';"},
		{microflows.LogLevelError, "'Orders'", "log error node 'Orders' 'Hello';"},
	} {
		got := formatAction(nil, &microflows.LogMessageAction{LogLevel: tc.level, LogNodeName: tc.node, MessageTemplate: msg}, nil, nil)
		if got != tc.want {
			t.Errorf("level %q node %q: got %q, want %q", tc.level, tc.node, got, tc.want)
		}
		// Read back: the omitted parts are the builder's defaults.
		prog, errs := visitor.Build("create microflow M.F () begin " + got + " end;")
		if len(errs) > 0 {
			t.Fatalf("%q does not parse: %v", got, errs)
		}
		body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
		log := body[0].(*ast.LogStmt)
		if (log.Node == nil) != (tc.node == "" || tc.node == defaultLogNodeExpression) {
			t.Errorf("%q: node read back as %v", got, log.Node)
		}
	}
}

// sortFixture is M.Order (extends M.Base) with Name, and M.Base with Code.
func sortFixture(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	base := mkEntity(mod.ID, "Base")
	base.Attributes = []*domainmodel.Attribute{{Name: "Code"}}
	order := mkEntity(mod.ID, "Order")
	order.GeneralizationRef = "M.Base"
	order.Attributes = []*domainmodel.Attribute{{Name: "Name"}}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{base, order},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func retrieveSorted(attr string, steps ...microflows.EntityRefStep) *microflows.RetrieveAction {
	return &microflows.RetrieveAction{
		OutputVariable: "Orders",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "M.Order",
			Sorting: []*microflows.SortItem{{
				AttributeQualifiedName: attr,
				EntityRefSteps:         steps,
				Direction:              microflows.SortDirectionAscending,
			}},
		},
	}
}

func TestFormatAction_SortByShortWhenUnambiguous(t *testing.T) {
	ctx := sortFixture(t)
	for _, tc := range []struct {
		name string
		act  *microflows.RetrieveAction
		want string
	}{
		{"own attribute", retrieveSorted("M.Order.Name"), "sort by Name asc"},
		// The builder qualifies a bare name with the DECLARING entity, so an
		// inherited attribute reads back to the same reference.
		{"inherited attribute", retrieveSorted("M.Base.Code"), "sort by Code asc"},
		// Not an attribute the retrieved entity has: the short form would be
		// qualified with M.Order and name something else.
		{"unknown attribute", retrieveSorted("M.Other.Name"), "sort by M.Other.Name asc"},
		// A path keeps its qualified end; the hop decides the entity.
		{"association path", retrieveSorted("M.Base.Code", microflows.EntityRefStep{Association: "M.Order_Base"}),
			"sort by M.Order_Base/M.Base.Code asc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := formatAction(ctx, tc.act, nil, nil)
			if !strings.Contains(got, tc.want+";") {
				t.Errorf("got %q, want it to end with %q", got, tc.want)
			}
		})
	}

	// Without a project nothing can be resolved, so nothing is shortened.
	if got := formatAction(nil, retrieveSorted("M.Order.Name"), nil, nil); !strings.Contains(got, "sort by M.Order.Name asc") {
		t.Errorf("disconnected describe shortened the sort: %q", got)
	}
}

func TestEmitObjectAnnotations_IfCaptionEqualToConditionIsOmitted(t *testing.T) {
	split := func(caption, expr string) *microflows.ExclusiveSplit {
		return &microflows.ExclusiveSplit{
			BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: nextID("split")}},
			Caption:             caption,
			SplitCondition:      &microflows.ExpressionSplitCondition{Expression: expr},
		}
	}
	emit := func(s *microflows.ExclusiveSplit) string {
		var lines []string
		emitObjectAnnotations(s, &lines, "", nil, nil, nil, nil)
		return strings.Join(lines, "\n")
	}
	if got := emit(split("$N > 10", "$N > 10")); strings.Contains(got, "@caption") {
		t.Errorf("caption equal to its condition was printed:\n%s", got)
	}
	if got := emit(split("Many open?", "$N > 10")); !strings.Contains(got, "@caption 'Many open?'") {
		t.Errorf("an authored caption was dropped:\n%s", got)
	}
}
