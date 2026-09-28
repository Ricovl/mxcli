// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `create or modify external entity` carried an attribute's stored RemoteType by
// name even when the statement declared another type: `OrderId: String(20)` kept
// Edm.Int64, and mx check then reported CE6616 "Attribute 'OrderId' is of type
// 'String' which is not the same as property type in the OData service which is
// 'Edm.Int64'" (measured on ako/TestApp, 11.14.0; ako/mxcli#764). The type of a
// mapped attribute is the service contract's, not the script's, so deriving a
// RemoteType from the declared type would only move the mismatch. Under mdl 1 the
// statement is refused, with nothing written; under mdl 0 it keeps writing, as
// it did, and warns MDL-V1-REMOTETYPE (ADR-0011: a new refusal applies only
// under the header that opts into it).

func remoteTypeCtx(t *testing.T, v langver.Version) (*ExecContext, *bytes.Buffer, **domainmodel.Entity) {
	t.Helper()
	mod := mkModule("Clients")
	h := mkHierarchy(mod)
	svc := &model.ConsumedODataService{BaseElement: model.BaseElement{ID: nextID("cos")}, ContainerID: mod.ID, Name: "OrderODataClient"}
	withContainer(h, svc.ContainerID, mod.ID)
	existing := &domainmodel.Entity{
		BaseElement:       model.BaseElement{ID: nextID("ent")},
		Name:              "Orders",
		Source:            "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "Clients.OrderODataClient",
		RemoteEntitySet:   "Orders",
		Attributes: []*domainmodel.Attribute{{
			BaseElement: model.BaseElement{ID: "stored-attr-id"},
			Name:        "OrderId",
			Type:        &domainmodel.LongAttributeType{},
			RemoteName:  "OrderId", RemoteType: "Edm.Int64", Filterable: true,
		}},
	}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: nextID("dm")}, ContainerID: mod.ID, Entities: []*domainmodel.Entity{existing}}
	var updated *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListModulesFunc:               func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:            func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) { return []*model.ConsumedODataService{svc}, nil },
		UpdateEntityFunc:              func(_ model.ID, e *domainmodel.Entity) error { updated = e; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	ctx.LanguageVersion = v
	return ctx, buf, &updated
}

func orderIDStmt(kind ast.DataTypeKind, length int) *ast.CreateExternalEntityStmt {
	return &ast.CreateExternalEntityStmt{
		Name:           ast.QualifiedName{Module: "Clients", Name: "Orders"},
		ServiceRef:     ast.QualifiedName{Module: "Clients", Name: "OrderODataClient"},
		Attributes:     []ast.Attribute{{Name: "OrderId", Type: ast.DataType{Kind: kind, Length: length}}},
		CreateOrModify: true,
	}
}

func TestCreateOrModifyExternalEntity_RemoteTypeMismatchRefusedUnderMdl1(t *testing.T) {
	ctx, _, updated := remoteTypeCtx(t, langver.V1)
	err := execCreateExternalEntity(ctx, orderIDStmt(ast.TypeString, 20))
	if err == nil {
		t.Fatal("a declared type the service does not publish was written under mdl 1")
	}
	for _, want := range []string{"OrderId", "Edm.Int64", "Long"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if *updated != nil {
		t.Error("the refused statement wrote the entity")
	}
}

func TestCreateOrModifyExternalEntity_RemoteTypeMismatchWarnsUnderMdl0(t *testing.T) {
	ctx, buf, updated := remoteTypeCtx(t, langver.V0)
	assertNoError(t, execCreateExternalEntity(ctx, orderIDStmt(ast.TypeString, 20)))
	if *updated == nil {
		t.Fatal("mdl 0 no longer writes the statement")
	}
	if !strings.Contains(buf.String(), "MDL-V1-REMOTETYPE") {
		t.Errorf("no MDL-V1-REMOTETYPE warning under mdl 0:\n%s", buf.String())
	}
}

// Control: the type the service publishes is carried without a word.
func TestCreateOrModifyExternalEntity_MatchingTypeCarriesRemoteType(t *testing.T) {
	for _, v := range []langver.Version{langver.V0, langver.V1} {
		ctx, buf, updated := remoteTypeCtx(t, v)
		assertNoError(t, execCreateExternalEntity(ctx, orderIDStmt(ast.TypeLong, 0)))
		if *updated == nil || (*updated).Attributes[0].RemoteType != "Edm.Int64" {
			t.Errorf("%s: RemoteType not carried: %+v", v, *updated)
		}
		if strings.Contains(buf.String(), "MDL-V1-REMOTETYPE") {
			t.Errorf("%s: warned on a matching type:\n%s", v, buf.String())
		}
	}
}
