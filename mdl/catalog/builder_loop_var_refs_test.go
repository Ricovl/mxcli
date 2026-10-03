// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `delete $Order` / `change $Order` inside `loop $Order in $Orders` wrote no refs
// row: the variable→entity map was seeded only from parameters and
// create/database-retrieve outputs, so a loop iterator — the commonest object
// variable in a batch flow — resolved to nothing, while `delete $Orders` on the
// list itself did produce the edge. An association retrieve's output was not
// mapped either. Side note on mendixlabs/mxcli#1266.

func iterLoop(id, list, iter string, children ...microflows.MicroflowObject) *microflows.LoopedActivity {
	l := newLoop(id, children...)
	l.LoopSource = &microflows.IterableList{ListVariableName: list, VariableName: iter}
	return l
}

func listParam(name, entity string) *microflows.MicroflowParameter {
	return &microflows.MicroflowParameter{Name: name, Type: &microflows.ListType{EntityQualifiedName: entity}}
}

func TestBuildVarEntityMap_LoopIteratorAndAssociationRetrieve(t *testing.T) {
	ends := map[string]assocEnds{"T.Order_Customer": {From: "T.Order", To: "T.Customer"}}
	oc := &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
		iterLoop("l1", "$Orders", "Order",
			// Forward: Order (FROM) over Order_Customer gives the Customer.
			newAction("r1", &microflows.RetrieveAction{
				OutputVariable: "Cust",
				Source:         &microflows.AssociationRetrieveSource{StartVariable: "Order", AssociationQualifiedName: "T.Order_Customer"},
			}),
			// Reverse: Customer (TO) over the same association gives its Orders,
			// iterated by a nested loop.
			newAction("r2", &microflows.RetrieveAction{
				OutputVariable: "CustOrders",
				Source:         &microflows.AssociationRetrieveSource{StartVariable: "$Cust", AssociationQualifiedName: "T.Order_Customer"},
			}),
			iterLoop("l2", "CustOrders", "Sibling"),
			// Unknown association: left unmapped rather than guessed.
			newAction("r3", &microflows.RetrieveAction{
				OutputVariable: "Mystery",
				Source:         &microflows.AssociationRetrieveSource{StartVariable: "Order", AssociationQualifiedName: "T.Unknown"},
			}),
		),
		// A loop over a list whose entity is unknown maps nothing.
		iterLoop("l3", "$Untyped", "X"),
	}}
	got := buildVarEntityMap([]*microflows.MicroflowParameter{listParam("Orders", "T.Order")}, oc, ends)
	want := map[string]string{
		"Orders": "T.Order", "Order": "T.Order",
		"Cust": "T.Customer", "CustOrders": "T.Order", "Sibling": "T.Order",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("varEntity[%q] = %q, want %q", k, got[k], v)
		}
	}
	for _, k := range []string{"Mystery", "X"} {
		if qn, ok := got[k]; ok {
			t.Errorf("varEntity[%q] = %q, want unmapped", k, qn)
		}
	}
}

// loopRefsCatalog runs the real full catalog build over one microflow and one
// nanoflow that change/delete their loop iterator, plus a control microflow that
// deletes the list itself (which always produced the edge).
func loopRefsCatalog(t *testing.T) *Catalog {
	t.Helper()
	const mod = model.ID("mod-t")
	orders := listParam("Orders", "T.Order")
	order := &domainmodel.Entity{Name: "Order"}
	order.ID = "ent-order"
	customer := &domainmodel.Entity{Name: "Customer"}
	customer.ID = "ent-customer"
	assoc := &domainmodel.Association{Name: "Order_Customer", ParentID: order.ID, ChildID: customer.ID}
	assoc.ID = "assoc-1"
	dm := &domainmodel.DomainModel{ContainerID: mod, Entities: []*domainmodel.Entity{order, customer}, Associations: []*domainmodel.Association{assoc}}
	dm.ID = "dm-t"

	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "T"}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListRulesFunc:        func() ([]*microflows.Rule, error) { return nil, nil },
		GetNavigationFunc:    func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{
					BaseElement: model.BaseElement{ID: "mf-loop"}, ContainerID: mod, Name: "DeleteEach",
					Parameters: []*microflows.MicroflowParameter{orders},
					ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
						iterLoop("l1", "Orders", "Order",
							newAction("d1", &microflows.DeleteObjectAction{DeleteVariable: "Order"}),
							newAction("r1", &microflows.RetrieveAction{
								OutputVariable: "Cust",
								Source:         &microflows.AssociationRetrieveSource{StartVariable: "Order", AssociationQualifiedName: "T.Order_Customer"},
							}),
							newAction("d2", &microflows.DeleteObjectAction{DeleteVariable: "Cust"}),
						),
					}},
				},
				{
					BaseElement: model.BaseElement{ID: "mf-ctl"}, ContainerID: mod, Name: "DeleteList",
					Parameters: []*microflows.MicroflowParameter{orders},
					ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
						iterLoop("l2", "Orders", "Order",
							newAction("d3", &microflows.DeleteObjectAction{DeleteVariable: "Orders"}),
						),
					}},
				},
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{{
				BaseElement: model.BaseElement{ID: "nf-loop"}, ContainerID: mod, Name: "NF_ChangeEach",
				Parameters: []*microflows.MicroflowParameter{orders},
				ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
					iterLoop("l3", "Orders", "Order",
						newAction("c1", &microflows.ChangeObjectAction{ChangeVariable: "Order"}),
					),
				}},
			}}, nil
		},
	}
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}
	return cat
}

func TestLoopIteratorChangeDeleteEmitRefs(t *testing.T) {
	cat := loopRefsCatalog(t)
	got := map[[3]string]bool{}
	for _, row := range queryRows(t, cat, `SELECT SourceName, TargetName, RefKind FROM refs WHERE TargetType = 'ENTITY' AND RefKind IN ('change', 'delete')`) {
		got[[3]string{row[0].(string), row[1].(string), row[2].(string)}] = true
	}
	// Control: the list delete always resolved. Without it, an empty refs table
	// (a build that wrote nothing) would fail the assertions below for the wrong
	// reason — and pass none.
	if !got[[3]string{"T.DeleteList", "T.Order", "delete"}] {
		t.Fatalf("control: `delete $Orders` must emit a delete edge; got %v", got)
	}
	for _, w := range [][3]string{
		{"T.DeleteEach", "T.Order", "delete"},    // delete of the loop iterator
		{"T.DeleteEach", "T.Customer", "delete"}, // delete of an association-retrieve output
		{"T.NF_ChangeEach", "T.Order", "change"}, // nanoflow: change of the loop iterator
	} {
		if !got[w] {
			t.Errorf("missing refs row %v; got %v", w, got)
		}
	}
}
