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

// systemDateIndexFixture is an entity storing changedDate with one index on
// that system member, descending — the shape the Siemens FSM template holds on
// ten entities (mendixlabs/mxcli#1401). Studio Pro stores such a segment as
// IndexedAttribute.Type = "ChangedDate" with no attribute behind its
// AttributePointer, because changedDate is not an attribute of the entity.
func systemDateIndexFixture(t *testing.T) (*ExecContext, *bytes.Buffer, *[]*domainmodel.Entity) {
	t.Helper()
	mod := mkModule("Repro")
	e := mkEntity(nextID("dm"), "E")
	e.HasChangedDate = true
	e.Attributes = []*domainmodel.Attribute{
		{BaseElement: model.BaseElement{ID: "attr-name"}, Name: "Name", Type: &domainmodel.StringAttributeType{Length: 200}},
	}
	seg := &domainmodel.IndexAttribute{AttributeID: "00000000-0000-0000-0000-000000000000", Ascending: false, Type: "ChangedDate"}
	seg.ID = nextID("ia")
	idx := &domainmodel.Index{Attributes: []*domainmodel.IndexAttribute{seg}}
	idx.ID = nextID("idx")
	e.Indexes = []*domainmodel.Index{idx}
	dm := mkDomainModel(mod.ID, e)

	var written []*domainmodel.Entity
	record := func(_ model.ID, e *domainmodel.Entity) error {
		written = append(written, e)
		return nil
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc:     record,
		UpdateEntityFunc:     record,
	}
	ctx, out := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, out, &written
}

// TestDescribeEntity_SystemDateIndexColumn_Issue1401: describe names the
// system member, and executing the output as a new entity writes the same
// index back — a ChangedDate segment, descending. Before the fix describe
// printed `index ( desc)`, which check refuses ("index on unknown attribute
// 'desc'"), and the create path dropped the column silently.
func TestDescribeEntity_SystemDateIndexColumn_Issue1401(t *testing.T) {
	ctx, out, _ := systemDateIndexFixture(t)
	if err := describeEntity(ctx, ast.QualifiedName{Module: "Repro", Name: "E"}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "( desc)") {
		t.Fatalf("describe emits an index with no column:\n%s", text)
	}
	if !strings.Contains(text, "index (ChangedDate desc)") {
		t.Fatalf("describe does not name the changedDate index column:\n%s", text)
	}

	// Execute the description as a NEW entity: the index comes from the text alone.
	script := strings.Replace(text, "entity Repro.E", "entity Repro.ECopy", 1)
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, script)
	}
	g, _, written := systemDateIndexFixture(t)
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateEntityStmt)
		if !ok {
			continue
		}
		if err := validate(g, s); err != nil {
			t.Fatalf("check refuses describe output: %v\n%s", err, script)
		}
		if err := execCreateEntity(g, s); err != nil {
			t.Fatalf("exec describe output: %v", err)
		}
	}
	if len(*written) == 0 {
		t.Fatal("nothing was written")
	}
	got := (*written)[len(*written)-1]
	if len(got.Indexes) != 1 || len(got.Indexes[0].Attributes) != 1 {
		t.Fatalf("round trip lost the index: %d indexes", len(got.Indexes))
	}
	seg := got.Indexes[0].Attributes[0]
	if seg.Type != "ChangedDate" || seg.Ascending {
		t.Errorf("round trip segment = {Type:%q Ascending:%v}, want {ChangedDate false}", seg.Type, seg.Ascending)
	}
}

// TestAlterEntityAddIndex_SystemDateColumn_Issue1401: the same column is
// reachable from `alter entity … add index`, and a second add is recognised as
// the existing index (CE0072 guard) rather than appended.
func TestAlterEntityAddIndex_SystemDateColumn_Issue1401(t *testing.T) {
	ctx, _, _ := systemDateIndexFixture(t)
	stmt := &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Repro", Name: "E"},
		Operation: ast.AlterEntityAddIndex,
		Index:     &ast.Index{Columns: []ast.IndexColumn{{Name: "ChangedDate", Descending: true}}},
	}
	err := execAlterEntity(ctx, stmt)
	if err == nil || !strings.Contains(err.Error(), "CE0072") {
		t.Fatalf("adding the stored changedDate index again must be refused as a duplicate, got: %v", err)
	}
}

// TestDescribeEntity_DanglingIndexColumnStillParses_Issue1401: a segment that
// names neither an attribute nor a system member has no MDL spelling. Describe
// says so in a comment and the statement still parses — the comment must not
// swallow the terminating semicolon.
func TestDescribeEntity_DanglingIndexColumnStillParses_Issue1401(t *testing.T) {
	ctx, out, _ := systemDateIndexFixture(t)
	dm, _ := ctx.Backend.GetDomainModel("")
	seg := dm.Entities[0].Indexes[0].Attributes[0]
	seg.Type = "Normal"
	seg.AttributeID = "attr-gone"
	if err := describeEntity(ctx, ast.QualifiedName{Module: "Repro", Name: "E"}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "( desc)") || strings.Contains(text, "\nindex (") {
		t.Fatalf("describe emits an index it cannot spell:\n%s", text)
	}
	prog, errs := visitor.Build(text)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, text)
	}
	if len(prog.Statements) == 0 {
		t.Fatalf("describe output parses to no statement:\n%s", text)
	}
}
