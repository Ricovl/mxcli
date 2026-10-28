// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#819, second item: `owner both` between a persistent and a
// non-persistent entity passed check and exec, and mxbuild then failed with
// CE0001 "An association between a persistable entity and a non-persistable
// entity must start in the non-persistable entity and have owner 'Default'."
// The table is what mxbuild 11.13.0 says, one association per blank app.
func TestAssociationPersistenceViolation_MatchesMxbuild(t *testing.T) {
	known := map[string]bool{"M.P": true, "M.P2": true, "M.N": false, "M.N2": false}
	persistable := func(qn ast.QualifiedName) (bool, bool) {
		p, ok := known[qn.String()]
		return p, ok
	}
	q := func(s string) ast.QualifiedName {
		i := strings.Index(s, ".")
		return ast.QualifiedName{Module: s[:i], Name: s[i+1:]}
	}
	for _, tc := range []struct {
		from, to  string
		ownerBoth bool
		ce0001    bool
	}{
		{"M.N", "M.P", false, false}, // the one shape Mendix allows
		{"M.N", "M.P", true, true},   // the reported case
		{"M.P", "M.N", false, true},
		{"M.P", "M.N", true, true},
		// Controls: no persistence boundary, so owner Both is fine.
		{"M.P", "M.P2", true, false},
		{"M.N", "M.N2", true, false},
		// An end nothing resolves is not judged.
		{"M.N", "M.Unknown", true, false},
	} {
		v := associationPersistenceViolation(q("M.A"), q(tc.from), q(tc.to), tc.ownerBoth, persistable)
		if (v != nil) != tc.ce0001 {
			t.Errorf("from %s to %s, owner both=%v: violation=%v, want %v", tc.from, tc.to, tc.ownerBoth, v != nil, tc.ce0001)
		}
		if v != nil && (v.RuleID != associationPersistenceRule || !strings.Contains(v.Message, "CE0001")) {
			t.Errorf("violation does not name the rule and CE0001: %+v", v)
		}
	}
}

// `check` without a project: the script's own declarations decide, including a
// specialization, which is as persistable as its generalization.
func TestValidateProgram_RefusesOwnerBothAcrossPersistenceFromTheScript(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"owner both", `create non-persistent entity M.Cart (Label: string(50));
create persistent entity M.Customer (Name: string(50));
create association M.Cart_Customer from M.Cart to M.Customer owner both;`, 1},
		{"persistent FROM", `create non-persistent entity M.Cart (Label: string(50));
create persistent entity M.Customer (Name: string(50));
create association M.Customer_Cart from M.Customer to M.Cart;`, 1},
		{"specialization of a non-persistent entity", `create non-persistent entity M.Cart (Label: string(50));
create entity M.BigCart extends M.Cart (Size: integer);
create persistent entity M.Customer (Name: string(50));
create association M.BigCart_Customer from M.BigCart to M.Customer owner both;`, 1},
		{"alter to owner both", `create non-persistent entity M.Cart (Label: string(50));
create persistent entity M.Customer (Name: string(50));
create association M.Cart_Customer from M.Cart to M.Customer;
alter association M.Cart_Customer set owner both;`, 1},
		// Controls.
		{"owner default", `create non-persistent entity M.Cart (Label: string(50));
create persistent entity M.Customer (Name: string(50));
create association M.Cart_Customer from M.Cart to M.Customer;`, 0},
		{"both persistent", `create persistent entity M.Order (Number: string(50));
create persistent entity M.Customer (Name: string(50));
create association M.Order_Customer from M.Order to M.Customer owner both;`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			for _, v := range ValidateProgram(parseMDL(t, tc.src), "") {
				if v.RuleID == associationPersistenceRule {
					n++
				}
			}
			if n != tc.want {
				t.Fatalf("%d %s violation(s), want %d", n, associationPersistenceRule, tc.want)
			}
		})
	}
}

// persistenceFixture: module M with the non-persistent Cart and the persistent
// Customer, and the stored association M.Cart_Customer (Cart → Customer, owner
// Default) — behind a mock that records what was written.
func persistenceFixture(t *testing.T) (*ExecContext, *mock.MockBackend, *int) {
	t.Helper()
	mod := mkModule("M")
	cart, customer := mkEntity(mod.ID, "Cart"), mkEntity(mod.ID, "Customer")
	cart.Persistable = false
	dm := mkDomainModel(mod.ID, cart, customer)
	dm.Associations = []*domainmodel.Association{mkAssociation(mod.ID, "Cart_Customer", cart.ID, customer.ID)}
	writes := 0
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		CreateAssociationFunc: func(model.ID, *domainmodel.Association) error {
			writes++
			return nil
		},
		UpdateDomainModelFunc: func(*domainmodel.DomainModel) error {
			writes++
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, mb, &writes
}

// `exec` refuses before writing — `check` is skipped under --no-check.
func TestExecAssociation_RefusesOwnerBothAcrossPersistence(t *testing.T) {
	q := func(name string) ast.QualifiedName { return ast.QualifiedName{Module: "M", Name: name} }

	t.Run("create", func(t *testing.T) {
		ctx, _, writes := persistenceFixture(t)
		err := execCreateAssociation(ctx, &ast.CreateAssociationStmt{
			Name: q("Cart_Customer2"), Parent: q("Cart"), Child: q("Customer"), Owner: ast.OwnerBoth,
		})
		if err == nil || !strings.Contains(err.Error(), "CE0001") {
			t.Fatalf("err = %v, want the CE0001 refusal", err)
		}
		if *writes != 0 {
			t.Errorf("%d write(s) before the refusal", *writes)
		}
	})
	t.Run("create or modify", func(t *testing.T) {
		ctx, _, writes := persistenceFixture(t)
		s := &ast.CreateAssociationStmt{
			Name: q("Cart_Customer"), Parent: q("Cart"), Child: q("Customer"), Owner: ast.OwnerBoth,
		}
		s.CreateOrModify = true
		if err := execCreateAssociation(ctx, s); err == nil || !strings.Contains(err.Error(), "CE0001") {
			t.Fatalf("err = %v, want the CE0001 refusal", err)
		}
		if *writes != 0 {
			t.Errorf("%d write(s) before the refusal", *writes)
		}
	})
	t.Run("alter set owner both", func(t *testing.T) {
		ctx, _, writes := persistenceFixture(t)
		err := execAlterAssociation(ctx, &ast.AlterAssociationStmt{
			Name: q("Cart_Customer"), Operation: ast.AlterAssociationSetOwner, Owner: ast.OwnerBoth,
		})
		if err == nil || !strings.Contains(err.Error(), "CE0001") {
			t.Fatalf("err = %v, want the CE0001 refusal", err)
		}
		if *writes != 0 {
			t.Errorf("%d write(s) before the refusal", *writes)
		}
	})
	// Control: the shape Mendix allows is written.
	t.Run("owner default", func(t *testing.T) {
		ctx, _, writes := persistenceFixture(t)
		assertNoError(t, execCreateAssociation(ctx, &ast.CreateAssociationStmt{
			Name: q("Cart_Customer2"), Parent: q("Cart"), Child: q("Customer"),
		}))
		if *writes != 1 {
			t.Errorf("%d write(s), want 1", *writes)
		}
	})
}

// The project half of `check`: an entity the script does not declare is judged
// by what the project stores.
func TestValidateAssociationPersistence_ResolvesTheProject(t *testing.T) {
	_, mb, _ := persistenceFixture(t)
	project := func() backend.FullBackend { return mb }
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"create association M.Cart_Customer2 from M.Cart to M.Customer owner both;", 1},
		{"alter association M.Cart_Customer set owner both;", 1},
		{"create association M.Cart_Customer2 from M.Cart to M.Customer;", 0},
	} {
		if got := len(ValidateAssociationPersistence(parseMDL(t, tc.src), project)); got != tc.want {
			t.Errorf("%q: %d violation(s), want %d", tc.src, got, tc.want)
		}
	}
}

// ako/mxcli#819, first item: `create or modify association` moving a
// cross-module association's TO end from one module to another reconciled no
// module but the declaring one. The old TO entity kept its OWNER Both entry and
// the new one never got it — CE0066 at both modules.
func TestCreateOrModifyAssociation_MovingTheToEndReconcilesBothToModules(t *testing.T) {
	for _, tc := range []struct {
		name string
		to   ast.QualifiedName
		want []string
	}{
		{"to another module", ast.QualifiedName{Module: "Hr", Name: "Employee"}, []string{"Crm", "Hr", "Shop"}},
		{"to another entity of the same module", ast.QualifiedName{Module: "Shop", Name: "Customer"}, []string{"Crm", "Shop"}},
		// Owner Default → Both without a move: the TO module still needs it.
		{"no move", ast.QualifiedName{Module: "Shop", Name: "Order"}, []string{"Crm", "Shop"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, reconciled, _ := dropEntityFixture(t)
			s := &ast.CreateAssociationStmt{
				Name:   ast.QualifiedName{Module: "Crm", Name: "Lead_Order"},
				Parent: ast.QualifiedName{Module: "Crm", Name: "Lead"},
				Child:  tc.to,
				Owner:  ast.OwnerBoth,
			}
			s.CreateOrModify = true
			assertNoError(t, execCreateAssociation(ctx, s))
			got := append([]string(nil), *reconciled...)
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("reconciled modules %v, want %v", got, tc.want)
			}
		})
	}
}
