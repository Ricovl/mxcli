// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#1449 (mendixlabs/mxcli#1551): `drop entity` removes every
// association with the entity at either end — the backend cascades into other
// modules' domain models too — but reconciled no access rule. The member entry
// for each removed association stayed on the entity at the OTHER end, and mx
// check failed with CE1613 "The selected association 'Shop.Order_Customer' no
// longer exists" at the access rule of that entity. `drop association` on the
// same association reconciles; `drop entity` skipped it.
//
// The rule that keeps the stale entry can live in a module other than the
// dropped entity's: the FROM entity of a cross-module association TO it, or the
// TO entity of an OWNER Both cross-module association FROM it. Each of those
// modules has to be reconciled, not only the dropped entity's own.

// dropEntityFixture: module Shop holds Order, Customer and Lonely, with
// Shop.Order_Customer (Order → Customer) and the cross-module Shop.Order_Contact
// (Order → Crm.Contact). Module Crm holds Contact and Lead, with the cross-module
// Crm.Lead_Order (Lead → Shop.Order) under OWNER Both. Hr is unrelated.
func dropEntityFixture(t *testing.T) (*ExecContext, *[]string, func() string) {
	t.Helper()
	shop, crm, hr := mkModule("Shop"), mkModule("Crm"), mkModule("Hr")
	order, customer, lonely := mkEntity(shop.ID, "Order"), mkEntity(shop.ID, "Customer"), mkEntity(shop.ID, "Lonely")
	contact, lead := mkEntity(crm.ID, "Contact"), mkEntity(crm.ID, "Lead")
	employee := mkEntity(hr.ID, "Employee")

	shopDM := mkDomainModel(shop.ID, order, customer, lonely)
	shopDM.Associations = []*domainmodel.Association{mkAssociation(shop.ID, "Order_Customer", order.ID, customer.ID)}
	orderContact := &domainmodel.CrossModuleAssociation{Name: "Order_Contact", ParentID: order.ID, ChildRef: "Crm.Contact",
		Type: "Reference", Owner: "Default"}
	orderContact.ID = nextID("xassoc")
	shopDM.CrossAssociations = []*domainmodel.CrossModuleAssociation{orderContact}

	crmDM := mkDomainModel(crm.ID, contact, lead)
	leadOrder := &domainmodel.CrossModuleAssociation{Name: "Lead_Order", ParentID: lead.ID, ChildRef: "Shop.Order",
		Type: "Reference", Owner: "Both"}
	leadOrder.ID = nextID("xassoc")
	crmDM.CrossAssociations = []*domainmodel.CrossModuleAssociation{leadOrder}

	hrDM := mkDomainModel(hr.ID, employee)

	dms := map[model.ID]*domainmodel.DomainModel{shop.ID: shopDM, crm.ID: crmDM, hr.ID: hrDM}
	mods := []*model.Module{shop, crm, hr}

	var reconciled []string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return mods, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			return dms[id], nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{shopDM, crmDM, hrDM}, nil
		},
		DeleteEntityFunc: func(model.ID, model.ID) error { return nil },
		ReconcileMemberAccessesFunc: func(_ model.ID, moduleName string) (int, error) {
			reconciled = append(reconciled, moduleName)
			return 1, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mods...)))
	return ctx, &reconciled, buf.String
}

func TestExecDropEntity_ReconcilesEveryModuleHoldingAMemberOfItsAssociations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entity ast.QualifiedName
		want   []string // modules whose access rules are reconciled
	}{
		// The reported case: Order's rule names Shop.Order_Customer.
		{"same-module TO end", ast.QualifiedName{Module: "Shop", Name: "Customer"}, []string{"Shop"}},
		// Reported too: the rule naming Shop.Order_Contact is Shop.Order's, in the
		// module that is NOT the dropped entity's.
		{"cross-module TO end", ast.QualifiedName{Module: "Crm", Name: "Contact"}, []string{"Crm", "Shop"}},
		// OWNER Both: Shop.Order's rule names Crm.Lead_Order as its TO end.
		{"cross-module FROM end, owner Both", ast.QualifiedName{Module: "Crm", Name: "Lead"}, []string{"Crm", "Shop"}},
		{"FROM end of both kinds", ast.QualifiedName{Module: "Shop", Name: "Order"}, []string{"Crm", "Shop"}},
		// Control: no association, nothing in another module to reconcile.
		{"no associations", ast.QualifiedName{Module: "Shop", Name: "Lonely"}, []string{"Shop"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reconciled, out := dropEntityFixture(t)
			assertNoError(t, execDropEntity(ctx, &ast.DropEntityStmt{Name: tc.entity}))
			got := append([]string(nil), *reconciled...)
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("reconciled modules %v, want %v — a module left out keeps a member for an association that no longer exists (CE1613)", got, tc.want)
			}
			if !strings.Contains(out(), "Reconciled 1 access rule(s) in module "+tc.want[0]) {
				t.Errorf("output does not report the reconcile: %q", out())
			}
			if !strings.Contains(out(), "Dropped entity: "+tc.entity.String()) {
				t.Errorf("output does not report the drop: %q", out())
			}
		})
	}
}

// The `drop association` half of the same defect: a cross-module association
// under OWNER Both is a member of its TO entity, whose rules live in the other
// module, and only the declaring module was reconciled.
func TestExecDropAssociation_CrossModuleReconcilesTheToModule(t *testing.T) {
	ctx, reconciled, _ := dropEntityFixture(t)
	mb := ctx.Backend.(*mock.MockBackend)
	mb.DeleteCrossAssociationFunc = func(model.ID, model.ID) error { return nil }

	assertNoError(t, execDropAssociation(ctx, &ast.DropAssociationStmt{
		Name: ast.QualifiedName{Module: "Crm", Name: "Lead_Order"},
	}))
	got := append([]string(nil), *reconciled...)
	sort.Strings(got)
	if strings.Join(got, ",") != "Crm,Shop" {
		t.Fatalf("reconciled modules %v, want [Crm Shop] — Shop.Order keeps Crm.Lead_Order (CE1613)", got)
	}
}
