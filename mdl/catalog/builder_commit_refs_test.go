// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `commit $Order` on a loop iterator wrote no refs row: refs had no commit kind
// at all (ako/mxcli#963). A flow that commits entity X now has a `commit` edge
// FLOW -> ENTITY, from a commit action and from a create or change that commits.
// A create/change with commit No is the control: it keeps its create/change edge
// and gains no commit edge.
func commitRefsCatalog(t *testing.T) *Catalog {
	t.Helper()
	const mod = model.ID("mod-t")
	orders := listParam("Orders", "T.Order")
	order := &domainmodel.Entity{Name: "Order"}
	order.ID = "ent-order"
	customer := &domainmodel.Entity{Name: "Customer"}
	customer.ID = "ent-customer"
	dm := &domainmodel.DomainModel{ContainerID: mod, Entities: []*domainmodel.Entity{order, customer}}
	dm.ID = "dm-t"
	flow := func(id, name string, objs ...microflows.MicroflowObject) *microflows.Microflow {
		return &microflows.Microflow{
			BaseElement: model.BaseElement{ID: model.ID(id)}, ContainerID: mod, Name: name,
			Parameters:       []*microflows.MicroflowParameter{orders},
			ObjectCollection: &microflows.MicroflowObjectCollection{Objects: objs},
		}
	}

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
				flow("mf-1", "CommitEach", iterLoop("l1", "Orders", "Order",
					newAction("a1", &microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true}))),
				flow("mf-2", "CommitList",
					newAction("a2", &microflows.CommitObjectsAction{CommitVariable: "$Orders"})),
				flow("mf-3", "CreateCommit",
					newAction("a3", &microflows.CreateObjectAction{EntityQualifiedName: "T.Customer", OutputVariable: "C", Commit: microflows.CommitTypeYes})),
				flow("mf-4", "ChangeCommit", iterLoop("l2", "Orders", "Order",
					newAction("a4", &microflows.ChangeObjectAction{ChangeVariable: "Order", Commit: microflows.CommitTypeYesWithoutEvents}))),
				flow("mf-5", "NoCommit",
					newAction("a5", &microflows.CreateObjectAction{EntityQualifiedName: "T.Customer", OutputVariable: "C", Commit: microflows.CommitTypeNo}),
					iterLoop("l3", "Orders", "Order",
						newAction("a6", &microflows.ChangeObjectAction{ChangeVariable: "Order", Commit: microflows.CommitTypeNo}))),
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{{
				BaseElement: model.BaseElement{ID: "nf-1"}, ContainerID: mod, Name: "NF_CommitEach",
				Parameters: []*microflows.MicroflowParameter{orders},
				ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
					iterLoop("l4", "Orders", "Order",
						newAction("a7", &microflows.CommitObjectsAction{CommitVariable: "Order"})),
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

func TestCommitEmitsCommitRefs(t *testing.T) {
	cat := commitRefsCatalog(t)
	var got []string
	for _, row := range queryRows(t, cat, `SELECT SourceName, TargetName, RefKind FROM refs
		WHERE TargetType = 'ENTITY' AND RefKind IN ('create', 'change', 'commit')`) {
		got = append(got, row[0].(string)+" "+row[2].(string)+" "+row[1].(string))
	}
	sort.Strings(got)
	want := []string{
		"T.ChangeCommit change T.Order",
		"T.ChangeCommit commit T.Order",
		"T.CommitEach commit T.Order", // commit of the loop iterator
		"T.CommitList commit T.Order",
		"T.CreateCommit commit T.Customer",
		"T.CreateCommit create T.Customer",
		"T.NF_CommitEach commit T.Order",
		// Control: create/change with commit No — no commit edge.
		"T.NoCommit change T.Order",
		"T.NoCommit create T.Customer",
	}
	if len(got) != len(want) {
		t.Fatalf("refs rows:\n%v\nwant:\n%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refs rows:\n%v\nwant:\n%v", got, want)
		}
	}
}
