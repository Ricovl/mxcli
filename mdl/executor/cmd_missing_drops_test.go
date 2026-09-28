// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// R6 (ako/mxcli#755): every document that can be created can be dropped. The
// three that could not: a validation rule, a database connection and an
// external entity.

func TestDropValidationRule_DropsOnlyTheNamedRule(t *testing.T) {
	required := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "Required"}
	regex := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "RegEx",
		Rule: &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Shop.EmailPattern"}}
	otherAttr := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Guests", Type: "Range",
		Rule: &domainmodel.RangeValidationRuleInfo{}}
	rangeOnEmail := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "Range",
		Rule: &domainmodel.RangeValidationRuleInfo{}}
	ctx, product := validationRuleCtx(t, required, regex, otherAttr, rangeOnEmail)

	assertNoError(t, execDropValidationRule(ctx, &ast.DropValidationRuleStmt{
		Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
		Kind:      ast.ValidationRuleRegEx,
	}))
	var got []string
	for _, vr := range product.ValidationRules {
		got = append(got, string(vr.AttributeID)+":"+vr.Type)
	}
	want := "Shop.Product.Email:Required Shop.Product.Guests:Range Shop.Product.Email:Range"
	if strings.Join(got, " ") != want {
		t.Errorf("rules left = %v, want %s", got, want)
	}
}

// Without a kind, both the regex and the range rule go; Required and Unique
// are attribute constraints (`not null`, `unique`) and stay.
func TestDropValidationRule_WithoutKindDropsRegexAndRange(t *testing.T) {
	required := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "Required"}
	regex := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "RegEx"}
	rng := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "Range"}
	ctx, product := validationRuleCtx(t, required, regex, rng)

	assertNoError(t, execDropValidationRule(ctx, &ast.DropValidationRuleStmt{
		Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
	}))
	if len(product.ValidationRules) != 1 || product.ValidationRules[0].Type != "Required" {
		t.Errorf("rules left = %+v, want only Required", product.ValidationRules)
	}
}

func TestDropValidationRule_NotFound(t *testing.T) {
	ctx, _ := validationRuleCtx(t)
	err := execDropValidationRule(ctx, &ast.DropValidationRuleStmt{
		Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
		Kind:      ast.ValidationRuleRange,
	})
	// A NotFoundError, so `drop validation rule if exists …` skips it.
	var nf *mdlerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
}

func dbConnectionCtx(t *testing.T, deleted *[]model.ID) *ExecContext {
	t.Helper()
	mod := mkModule("Shop")
	conn := &model.DatabaseConnection{ContainerID: mod.ID, Name: "Erp"}
	conn.ID = nextID("dbconn")
	other := &model.DatabaseConnection{ContainerID: mod.ID, Name: "Crm"}
	other.ID = nextID("dbconn")
	h := mkHierarchy(mod)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDatabaseConnectionsFunc: func() ([]*model.DatabaseConnection, error) {
			return []*model.DatabaseConnection{other, conn}, nil
		},
		DeleteDatabaseConnectionFunc: func(id model.ID) error { *deleted = append(*deleted, id); return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func TestDropDatabaseConnection(t *testing.T) {
	var deleted []model.ID
	ctx := dbConnectionCtx(t, &deleted)
	assertNoError(t, execDropDatabaseConnection(ctx, &ast.DropDatabaseConnectionStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Erp"},
	}))
	conns, _ := ctx.Backend.ListDatabaseConnections()
	if len(deleted) != 1 || deleted[0] != conns[1].ID {
		t.Errorf("deleted %v, want only Shop.Erp (%s)", deleted, conns[1].ID)
	}

	deleted = nil
	err := execDropDatabaseConnection(ctx, &ast.DropDatabaseConnectionStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Nope"},
	})
	var nf *mdlerrors.NotFoundError
	if !errors.As(err, &nf) || len(deleted) != 0 {
		t.Errorf("err = %v, deleted = %v; want not found and nothing deleted", err, deleted)
	}
}

func externalEntityCtx(t *testing.T, deleted *[]model.ID) (*ExecContext, *domainmodel.Entity, *domainmodel.Entity) {
	t.Helper()
	mod := mkModule("Shop")
	remote := &domainmodel.Entity{Name: "Customer", Source: "Rest$ODataRemoteEntitySource"}
	remote.ID = nextID("entity")
	local := &domainmodel.Entity{Name: "Order"}
	local.ID = nextID("entity")
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{remote, local},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		DeleteEntityFunc: func(_ model.ID, id model.ID) error {
			*deleted = append(*deleted, id)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, remote, local
}

func TestDropExternalEntity(t *testing.T) {
	var deleted []model.ID
	ctx, remote, local := externalEntityCtx(t, &deleted)

	assertNoError(t, execDropEntity(ctx, &ast.DropEntityStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Customer"}, External: true,
	}))
	if len(deleted) != 1 || deleted[0] != remote.ID {
		t.Fatalf("deleted %v, want the external entity %s", deleted, remote.ID)
	}

	// `drop external entity` on a local entity is refused: the word says
	// which kind the author means, so a wrong name is not silently dropped.
	deleted = nil
	err := execDropEntity(ctx, &ast.DropEntityStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Order"}, External: true,
	})
	if err == nil || !strings.Contains(err.Error(), "not an external entity") || len(deleted) != 0 {
		t.Errorf("err = %v, deleted = %v; want a refusal and nothing deleted", err, deleted)
	}
	// Control: plain `drop entity` still drops a local one.
	assertNoError(t, execDropEntity(ctx, &ast.DropEntityStmt{Name: ast.QualifiedName{Module: "Shop", Name: "Order"}}))
	if len(deleted) != 1 || deleted[0] != local.ID {
		t.Errorf("deleted %v, want the local entity", deleted)
	}
}
