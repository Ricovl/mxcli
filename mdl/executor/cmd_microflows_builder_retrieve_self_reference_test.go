// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1793: the full microflow build typed `retrieve $P from
// $Order/M.Order_Parent` — a Reference from M.Order to M.Order — as a LIST of
// M.Order unless $P was later used as an object, because the start variable is
// an instance of the association's TO entity (as well as its FROM entity), so
// the retrieve was classified as a reverse traversal. Mendix reads it the
// other way: the stored AssociationRetrieveSource records no direction and a
// self-reference is followed from its FROM end. Measured on 11.13.0 with
// M.Order_Parent (Reference, owner Default) and M.Order_Related (Reference
// set), each shape in its own microflow:
//
//	retrieve $P from $Order/M.Order_Parent; change $P (…)   0 errors
//	… $n = count $P;                CE0013 "Input variable 'P' must be of type 'List'."
//	  (sum, average, minimum, maximum, all, any, reduce: CE0013 too)
//	… $h = head $P;                 CE0097 "The selected 'P' variable must be of type List."
//	  (tail, find, filter, sort, union, intersect, subtract, contains, equals: CE0097 too)
//	… $r = range $P limit 1;        0 errors (Microflows$ListRange: mx check does not type it)
//	… add / remove / clear $P       0 errors (Change list: mx check does not type it)
//	… loop $x in $P                 CE0100 "'P' is of type M.Order, but should be of type List."
//	… call M.SubList(Orders = $P)   CE0117 "Error(s) in expression."
//	the same from a $S: M.SpecialOrder (a specialization)   the same
//	retrieve $R from $Order/M.Order_Related; count / loop   0 errors
//	… declare $c String = $R/Code;  CE0117 (the reference set is a list)
//	retrieve $C from $Order/M.Order_Customer; count $C      CE0013 (a forward Reference)
//
// check --references was silent on every one of them, so a list use was
// accepted, written by exec, and failed the build.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// selfReferenceBackend is a project with Sample.Node, which has a Reference
// (Node_Parent), a Reference set (Node_Related) and a one-to-one (Node_Twin,
// owner Both — CE0002 in Studio Pro, but a stored model is what the builder
// reads) from itself to itself, Sample.SpecialNode a specialization of it,
// Sample.Line with a Reference Line_Node to Node, and Sample.Animal with its
// specialization Sample.Dog, with a Reference Dog_Animal from Dog to Animal and
// Animal_Dog the other way: a Dog is a specialization of both ends of either.
func selfReferenceBackend() *mock.MockBackend {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-sample"}, Name: "Sample"}
	ent := func(id, name, general string) *domainmodel.Entity {
		return &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID(id)}, ContainerID: mod.ID,
			Name: name, Persistable: true, GeneralizationRef: general}
	}
	assoc := func(name, parent, child string, typ domainmodel.AssociationType, owner domainmodel.AssociationOwner) *domainmodel.Association {
		return &domainmodel.Association{BaseElement: model.BaseElement{ID: model.ID("a-" + name)}, ContainerID: mod.ID,
			Name: name, ParentID: model.ID(parent), ChildID: model.ID(child), Type: typ, Owner: owner}
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-sample"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{
			ent("e-node", "Node", ""),
			ent("e-special", "SpecialNode", "Sample.Node"),
			ent("e-line", "Line", ""),
			ent("e-animal", "Animal", ""),
			ent("e-dog", "Dog", "Sample.Animal"),
		},
		Associations: []*domainmodel.Association{
			assoc("Node_Parent", "e-node", "e-node", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault),
			assoc("Node_Related", "e-node", "e-node", domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerDefault),
			assoc("Node_Twin", "e-node", "e-node", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerBoth),
			assoc("Line_Node", "e-line", "e-node", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault),
			assoc("Dog_Animal", "e-dog", "e-animal", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault),
			assoc("Animal_Dog", "e-animal", "e-dog", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault),
		},
	}
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == mod.Name {
				return mod, nil
			}
			return nil, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == mod.ID {
				return dm, nil
			}
			return nil, nil
		},
	}
}

func selfReferenceFlowBuilder() *flowBuilder {
	return &flowBuilder{
		varTypes: map[string]string{
			"Node":    "Sample.Node",
			"Special": "Sample.SpecialNode",
			"Line":    "Sample.Line",
			"Dog":     "Sample.Dog",
		},
		backend: selfReferenceBackend(),
		posX:    200,
		posY:    200,
		spacing: HorizontalSpacing,
	}
}

// TestBuildFlowGraph_SelfReferenceIsFollowedFromItsFromEnd is the reported
// shape: a list use after a retrieve over a self-referencing Reference. The
// output is the one object the FROM end holds, whatever the use, and it stays
// an association retrieve — a self-reference never takes the owner-Both
// reverse fallback to a database retrieve.
func TestBuildFlowGraph_SelfReferenceIsFollowedFromItsFromEnd(t *testing.T) {
	count := func(list string) ast.MicroflowStatement {
		return &ast.AggregateListStmt{OutputVariable: "N", Operation: ast.AggregateCount, InputVariable: list}
	}
	change := func(obj string) ast.MicroflowStatement {
		return &ast.ChangeObjectStmt{Variable: obj, Changes: []ast.ChangeItem{{Attribute: "Name", Value: &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}}}}
	}
	loop := func(list string) ast.MicroflowStatement {
		return &ast.LoopStmt{LoopVariable: "It", ListVariable: list, Body: []ast.MicroflowStatement{}}
	}
	for _, tc := range []struct {
		name, start, assoc string
		use                ast.MicroflowStatement
		want               string
	}{
		{"Reference, a list use (count)", "Node", "Node_Parent", count("P"), "Sample.Node"},
		{"Reference, a list use (loop)", "Node", "Node_Parent", loop("P"), "Sample.Node"},
		{"Reference, unused", "Node", "Node_Parent", nil, "Sample.Node"},
		{"Reference, an object use", "Node", "Node_Parent", change("P"), "Sample.Node"},
		{"Reference from a specialization, a list use", "Special", "Node_Parent", count("P"), "Sample.Node"},
		{"owner Both, a list use: no database fallback", "Node", "Node_Twin", count("P"), "Sample.Node"},
		{"Reference set: the list", "Node", "Node_Related", count("P"), "List of Sample.Node"},
		{"Reference set from a specialization: the list", "Special", "Node_Related", nil, "List of Sample.Node"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := selfReferenceFlowBuilder()
			stmts := []ast.MicroflowStatement{&ast.RetrieveStmt{
				Variable:      "P",
				StartVariable: tc.start,
				Source:        ast.QualifiedName{Module: "Sample", Name: tc.assoc},
			}}
			if tc.use != nil {
				stmts = append(stmts, tc.use)
			}
			fb.buildFlowGraph(stmts, nil)

			action := firstRetrieveAction(t, fb)
			source, ok := action.Source.(*microflows.AssociationRetrieveSource)
			if !ok {
				t.Fatalf("source = %T, want an AssociationRetrieveSource", action.Source)
			}
			if source.StartVariable != tc.start || source.AssociationQualifiedName != "Sample."+tc.assoc {
				t.Fatalf("association source = %#v", source)
			}
			if got := fb.varTypes["P"]; got != tc.want {
				t.Errorf("$P typed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildFlowGraph_AssociationDirectionControls: the shapes the fix must not
// move. A Reference from its TO end is the list of FROM objects, and from its
// FROM end the one TO object; a start that is a specialization of both ends of
// an association between two different entities stays read from the TO end,
// as before #1793 (which end Mendix starts from there is not measured).
func TestBuildFlowGraph_AssociationDirectionControls(t *testing.T) {
	for _, tc := range []struct {
		name, start, assoc, want string
	}{
		{"Reference from its TO end", "Node", "Line_Node", "List of Sample.Line"},
		{"Reference from its FROM end", "Line", "Line_Node", "Sample.Node"},
		{"a specialization of both ends of two entities", "Dog", "Dog_Animal", "List of Sample.Dog"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := selfReferenceFlowBuilder()
			fb.buildFlowGraph([]ast.MicroflowStatement{
				&ast.RetrieveStmt{Variable: "P", StartVariable: tc.start, Source: ast.QualifiedName{Module: "Sample", Name: tc.assoc}},
				&ast.AggregateListStmt{OutputVariable: "N", Operation: ast.AggregateCount, InputVariable: "P"},
			}, nil)
			if got := fb.varTypes["P"]; got != tc.want {
				t.Errorf("$P typed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAssociationRetrieveType_AgreesWithTheFullBuild: the fragment a splice
// builds (associationRetrieveType, ako/mxcli#1777) and the full build
// (addRetrieveAction) take one rule for which end a retrieve starts from, so
// they type every unambiguous shape alike. The full build's type is read with
// no use following the retrieve, where it is the association's own answer.
func TestAssociationRetrieveType_AgreesWithTheFullBuild(t *testing.T) {
	for _, tc := range []struct{ start, assoc string }{
		{"Node", "Node_Parent"},
		{"Special", "Node_Parent"},
		{"Node", "Node_Related"},
		{"Special", "Node_Related"},
		{"Node", "Node_Twin"},
		{"Node", "Line_Node"},
		{"Line", "Line_Node"},
	} {
		fb := selfReferenceFlowBuilder()
		startType := fb.varTypes[tc.start]
		spliced := fb.associationRetrieveType(startType, "Sample."+tc.assoc)
		fb.addRetrieveAction(&ast.RetrieveStmt{Variable: "P", StartVariable: tc.start,
			Source: ast.QualifiedName{Module: "Sample", Name: tc.assoc}})
		if built := fb.varTypes["P"]; spliced == "" || built != spliced {
			t.Errorf("$%s/Sample.%s: the full build types %q, a splice %q", tc.start, tc.assoc, built, spliced)
		}
	}
	// The ambiguous shape is the one the splice leaves untyped.
	fb := selfReferenceFlowBuilder()
	if got := fb.associationRetrieveType("Sample.Dog", "Sample.Dog_Animal"); got != "" {
		t.Errorf("a specialization of both ends of two entities: a splice typed %q, want it untyped", got)
	}
}
