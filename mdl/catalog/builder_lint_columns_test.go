// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// lintColumnsBuilder is a Builder over one module, Sales, whose domain model
// carries documentation, a same-module association with a non-default delete
// behaviour on BOTH ends, and a cross-module one. A second module, Empty, has a
// domain model with no documentation -- the control for the module column.
func lintColumnsBuilder(t *testing.T) (*Builder, *Catalog) {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })

	const sales, empty = model.ID("mod-sales"), model.ID("mod-empty")
	order := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-order"}, Name: "Order", Persistable: true}
	line := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-line"}, Name: "OrderLine", Persistable: true}

	assoc := &domainmodel.Association{
		BaseElement: model.BaseElement{ID: "a-line-order"},
		Name:        "OrderLine_Order",
		ParentID:    "e-line",  // FROM: OrderLine owns the reference
		ChildID:     "e-order", // TO: Order is referenced
		Type:        domainmodel.AssociationTypeReference,
		Owner:       domainmodel.AssociationOwnerDefault,
		// Distinct values on each end, so a swapped mapping cannot pass.
		ChildDeleteBehavior: &domainmodel.DeleteBehavior{
			Type:         domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
			ErrorMessage: "Order still has lines",
		},
		ParentDeleteBehavior: &domainmodel.DeleteBehavior{
			Type:         domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
			ErrorMessage: "parent side message",
		},
	}
	cross := &domainmodel.CrossModuleAssociation{
		BaseElement: model.BaseElement{ID: "a-order-customer"},
		Name:        "Order_Customer",
		ParentID:    "e-order",
		ChildRef:    "CRM.Customer",
		Type:        domainmodel.AssociationTypeReference,
		Owner:       domainmodel.AssociationOwnerDefault,
		ChildDeleteBehavior: &domainmodel.DeleteBehavior{
			Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
		},
		ParentDeleteBehavior: &domainmodel.DeleteBehavior{
			Type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences,
		},
	}

	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListModulesFunc: func() ([]*model.Module, error) {
				s := &model.Module{Name: "Sales"}
				s.ID = sales
				e := &model.Module{Name: "Empty"}
				e.ID = empty
				return []*model.Module{s, e}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{sales: true, empty: true},
			moduleNames:     map[model.ID]string{sales: "Sales", empty: "Empty"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		domainModelCache: []*domainmodel.DomainModel{
			{
				ContainerID:       sales,
				Documentation:     "Orders and their lines.",
				Entities:          []*domainmodel.Entity{order, line},
				Associations:      []*domainmodel.Association{assoc},
				CrossAssociations: []*domainmodel.CrossModuleAssociation{cross},
			},
			{ContainerID: empty},
		},
		tx: tx,
	}
	return b, cat
}

// mendixlabs/mxcli#1269: the catalog had no delete behaviour, although the
// reader decodes both ends. Mendix's pointer names are inverted relative to
// MDL's FROM/TO (CLAUDE.md), so the Child* behaviour is the TO end's and the
// Parent* behaviour the FROM end's -- the fixture gives each end a different
// value so a swap fails rather than passes.
func TestAssociationsCarryDeleteBehaviourOfBothEnds(t *testing.T) {
	b, cat := lintColumnsBuilder(t)
	if err := b.buildAssociations(); err != nil {
		t.Fatalf("buildAssociations: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res, err := cat.Query(`SELECT QualifiedName, FromEntity, ToEntity,
		ToDeleteBehavior, ToDeleteErrorMessage, FromDeleteBehavior, FromDeleteErrorMessage
		FROM associations ORDER BY QualifiedName`)
	if err != nil {
		t.Fatalf("query: %v -- the columns must exist", err)
	}
	got := map[string][]any{}
	for _, row := range res.Rows {
		got[row[0].(string)] = row[1:]
	}
	want := map[string][]any{
		"Sales.OrderLine_Order": {"Sales.OrderLine", "Sales.Order",
			"DeleteMeIfNoReferences", "Order still has lines",
			"DeleteMeAndReferences", "parent side message"},
		// The cross-association branch writes the same columns.
		"Sales.Order_Customer": {"Sales.Order", "CRM.Customer",
			"DeleteMeAndReferences", "", "DeleteMeButKeepReferences", ""},
	}
	for qn, w := range want {
		g, ok := got[qn]
		if !ok {
			t.Errorf("no row for %s", qn)
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s column %d = %v, want %v (row %v)", qn, i, g[i], w[i], g)
			}
		}
	}
}

// mendixlabs/mxcli#1269: the domain model's documentation was read nowhere.
// The Empty module is the control: a module whose domain model has none must
// not borrow its neighbour's.
func TestModulesCarryDomainModelDocumentation(t *testing.T) {
	b, cat := lintColumnsBuilder(t)
	if err := b.buildModules(); err != nil {
		t.Fatalf("buildModules: %v", err)
	}
	if err := b.tx.Commit(); err != nil {
		t.Fatal(err)
	}
	docs := queryStrings(t, cat, `SELECT Name, DomainModelDocumentation FROM modules`)
	if got, want := docs["Sales"], "Orders and their lines."; got != want {
		t.Errorf("Sales domain model documentation = %q, want %q", got, want)
	}
	if got := docs["Empty"]; got != "" {
		t.Errorf("Empty domain model documentation = %q, want empty", got)
	}
}
