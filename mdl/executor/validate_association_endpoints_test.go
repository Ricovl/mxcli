// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#555: check --references resolved an association's endpoint
// MODULES and never the entities, so
//
//	from ServiceCore.WorkOrder to System.TotallyMadeUpEntity   -> Check passed
//	from ServiceCore.MadeUpChild to ServiceCore.Customer       -> Check passed
//
// and exec then stopped at the statement ("child entity not found") with the
// statements before it already written. Measured on 11.14.0: exec refuses both
// shapes; the control (two existing entities) is created.

func storedWorkOrder() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-wo", TypeName: "DomainModels$Entity"},
		Name:        "WorkOrder",
		Persistable: true,
	}
}

func storedCustomer() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-cust", TypeName: "DomainModels$Entity"},
		Name:        "Customer",
		Persistable: true,
	}
}

func endpointAssoc(from, to string) *ast.CreateAssociationStmt {
	return &ast.CreateAssociationStmt{
		Name:   ast.QualifiedName{Module: "ServiceCore", Name: "A_" + from + "_" + to},
		Parent: ast.QualifiedName{Module: "ServiceCore", Name: from},
		Child:  ast.QualifiedName{Module: "ServiceCore", Name: to},
		Type:   ast.AssocReference,
	}
}

func TestAssociationEndpoints_UnknownEntityIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, from, to, missing string
	}{
		{"unknown TO entity", "WorkOrder", "TotallyMadeUpEntity", "ServiceCore.TotallyMadeUpEntity"},
		{"unknown FROM entity", "MadeUpChild", "Customer", "ServiceCore.MadeUpChild"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, storedWorkOrder(), storedCustomer())
			err := validateWithContext(ctx, endpointAssoc(c.from, c.to), newScriptContext())
			if err == nil {
				t.Fatalf("association %s -> %s passed --references; exec stops on it", c.from, c.to)
			}
			if !strings.Contains(err.Error(), c.missing) {
				t.Errorf("the refusal should name %s: %v", c.missing, err)
			}
		})
	}
}

// The controls: both endpoints stored, or produced by the script — created,
// renamed into place, or imported by a bulk `create external entities` — must
// not be refused. A check louder than exec is a blocker now that exec refuses
// what check reports.
func TestAssociationEndpoints_ResolvableEndpointsPass(t *testing.T) {
	for _, c := range []struct {
		name  string
		from  string
		to    string
		setup ast.Statement
	}{
		{"both stored", "WorkOrder", "Customer", nil},
		{"created by the script", "WorkOrder", "Visit", &ast.CreateEntityStmt{
			Name: ast.QualifiedName{Module: "ServiceCore", Name: "Visit"}, Kind: ast.EntityPersistent}},
		{"renamed into place by the script", "WorkOrder", "Client", &ast.RenameStmt{
			ObjectType: "entity", Name: ast.QualifiedName{Module: "ServiceCore", Name: "Customer"}, NewName: "Client"}},
		{"imported by create external entities", "WorkOrder", "Order", &ast.CreateExternalEntitiesStmt{
			ServiceRef: ast.QualifiedName{Module: "ServiceCore", Name: "OrdersApi"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, storedWorkOrder(), storedCustomer())
			sc := newScriptContext()
			if c.setup != nil {
				sc.collectSingle(c.setup)
			}
			if err := validateWithContext(ctx, endpointAssoc(c.from, c.to), sc); err != nil {
				t.Fatalf("refused a resolvable association: %v", err)
			}
		})
	}
}

func TestAssociationEndpoints_SystemAccountHint(t *testing.T) {
	hint := entityNameHint(ast.QualifiedName{Module: "System", Name: "Account"})
	if !strings.Contains(hint, "System.User") {
		t.Errorf("System.Account should point at System.User, got %q", hint)
	}
}
