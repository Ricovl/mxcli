// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#885: a fragment spliced into a stored flow was built knowing only
// the types of the stored flow's parameters, declared variables and created
// objects, so a member of a variable bound by a retrieve or a microflow call
// was written bare — `Name`, not `Mod.Item.Name` — and Mendix cannot load the
// project. These tests build fragments the way the splice does.

// spliceFixture is a stored flow over Mod.Item (attribute Name) with a
// parameter $P, a database retrieve $R (first), a call $C of Mod.GetItem
// (returns Mod.Item), a call $Cs of Mod.GetItems (returns a list) and a retrieve
// over an association $A, whose entity the stored flow does not say.
func spliceFixture(t *testing.T) (*ExecContext, *alterFlowContext) {
	t.Helper()
	mod := mkModule("Mod")
	item := mkEntity(mod.ID, "Item")
	item.Attributes = []*domainmodel.Attribute{{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: "Name"}}
	dm := mkDomainModel(mod.ID, item)
	returns := map[string]microflows.DataType{
		"Mod.GetItem":  &microflows.ObjectType{EntityQualifiedName: "Mod.Item"},
		"Mod.GetItems": &microflows.ListType{EntityQualifiedName: "Mod.Item"},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Mod" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetRawUnitByNameFunc: func(_, qn string) (*types.RawUnitInfo, error) {
			if _, ok := returns[qn]; ok {
				return &types.RawUnitInfo{ID: qn, QualifiedName: qn, Contents: []byte(qn)}, nil
			}
			return nil, nil
		},
		ParseMicroflowBSONFunc: func(contents []byte, _, _ model.ID) (*microflows.Microflow, error) {
			return &microflows.Microflow{ReturnType: returns[string(contents)]}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	activity := func(a microflows.MicroflowAction) microflows.MicroflowObject {
		return &microflows.ActionActivity{Action: a}
	}
	call := func(qn string) microflows.MicroflowAction {
		return &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{Microflow: qn}}
	}
	a := &alterFlowContext{
		stmt: &ast.AlterFlowStmt{},
		mf: &microflows.Microflow{Parameters: []*microflows.MicroflowParameter{
			{Name: "P", Type: &microflows.ObjectType{EntityQualifiedName: "Mod.Item"}},
		}},
		entityNames: map[model.ID]string{item.ID: "Mod.Item"},
		cands: []mfmutator.Candidate{
			{OutputVariable: "R", Object: activity(&microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{
				EntityID: item.ID, Range: &microflows.Range{RangeType: microflows.RangeTypeFirst}}})},
			{OutputVariable: "C", Object: activity(call("Mod.GetItem"))},
			{OutputVariable: "Cs", Object: activity(call("Mod.GetItems"))},
			{OutputVariable: "A", Object: activity(&microflows.RetrieveAction{Source: &microflows.AssociationRetrieveSource{
				StartVariable: "P", AssociationQualifiedName: "Mod.Item_Item"}})},
		},
	}
	return ctx, a
}

func changeName(variable string) *ast.ChangeObjectStmt {
	return &ast.ChangeObjectStmt{Variable: variable, Changes: []ast.ChangeItem{
		{Attribute: "Name", Value: &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}},
	}}
}

// changedAttribute builds a one-change fragment and returns the attribute it
// writes, or the build error.
func changedAttribute(t *testing.T, ctx *ExecContext, a *alterFlowContext, variable string) (string, error) {
	t.Helper()
	frag, err := a.buildFragment(ctx, []ast.MicroflowStatement{changeName(variable)})
	if err != nil {
		return "", err
	}
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if ch, ok := act.Action.(*microflows.ChangeObjectAction); ok && len(ch.Changes) == 1 {
				return ch.Changes[0].AttributeQualifiedName, nil
			}
		}
	}
	t.Fatalf("the fragment of change $%s has no change activity", variable)
	return "", nil
}

func TestSpliceFragment_ChangeMemberIsQualified(t *testing.T) {
	ctx, a := spliceFixture(t)
	for _, v := range []string{
		"P", // control: a parameter was always typed
		"R", // a database retrieve
		"C", // a microflow call
	} {
		got, err := changedAttribute(t, ctx, a, v)
		if err != nil {
			t.Errorf("change $%s: %v", v, err)
			continue
		}
		if got != "Mod.Item.Name" {
			t.Errorf("change $%s writes attribute %q, want Mod.Item.Name", v, got)
		}
	}
}

// A variable the stored flow does not type — a retrieve over an association —
// is typed by the declared flow's full build when the statement is a
// `create or modify`, and refused, never written bare, when nothing types it.
func TestSpliceFragment_UntypedVariable(t *testing.T) {
	ctx, a := spliceFixture(t)
	if _, err := changedAttribute(t, ctx, a, "A"); err == nil || !strings.Contains(err.Error(), `cannot qualify member "Name"`) {
		t.Fatalf("change on an untyped variable: want a refusal naming the member, got %v", err)
	}

	a.declaredVarTypes = map[string]string{"A": "Mod.Item"}
	got, err := changedAttribute(t, ctx, a, "A")
	if err != nil || got != "Mod.Item.Name" {
		t.Fatalf("change on a variable the declared flow types: got %q, %v; want Mod.Item.Name", got, err)
	}
}

// The other activities that name a member of a list variable's entity refuse
// one they cannot qualify instead of writing it bare (a sort), as a find by
// expression (a find), or not at all (an aggregate). Control: over the typed
// list $Cs each builds.
func TestSpliceFragment_ListMembersAreRefusedWhenUntyped(t *testing.T) {
	ctx, a := spliceFixture(t)
	stmts := func(list string) map[string]ast.MicroflowStatement {
		return map[string]ast.MicroflowStatement{
			"sort": &ast.ListOperationStmt{OutputVariable: "S", Operation: ast.ListOpSort, InputVariable: list,
				SortSpecs: []ast.SortSpec{{Attribute: "Name", Ascending: true}}},
			"find": &ast.ListOperationStmt{OutputVariable: "F", Operation: ast.ListOpFind, InputVariable: list,
				Condition: &ast.BinaryExpr{Left: &ast.IdentifierExpr{Name: "Name"}, Operator: "=",
					Right: &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}}},
			"aggregate": &ast.AggregateListStmt{OutputVariable: "M", Operation: ast.AggregateMaximum, InputVariable: list,
				Attribute: "Name"},
		}
	}
	for name, st := range stmts("Cs") {
		if _, err := a.buildFragment(ctx, []ast.MicroflowStatement{st}); err != nil {
			t.Errorf("%s over a typed list: %v", name, err)
		}
	}
	a.cands = append(a.cands, mfmutator.Candidate{OutputVariable: "As", Object: &microflows.ActionActivity{
		Action: &microflows.RetrieveAction{Source: &microflows.AssociationRetrieveSource{StartVariable: "P"}}}})
	for name, st := range stmts("As") {
		if _, err := a.buildFragment(ctx, []ast.MicroflowStatement{st}); err == nil || !strings.Contains(err.Error(), "cannot qualify member") {
			t.Errorf("%s over an untyped list: want a refusal, got %v", name, err)
		}
	}
}
