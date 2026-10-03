// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A `HashedString` attribute was written as an unlimited String: the visitor
// built TypeString for it, and convertDataType had no case to map it to even
// once it did. These tests pin the executor half — the kind reaches the backend
// as a HashedStringAttributeType, and describe prints it back as HashedString.

func TestConvertDataType_HashedString(t *testing.T) {
	got := convertDataType(ast.DataType{Kind: ast.TypeHashedString})
	if _, ok := got.(*domainmodel.HashedStringAttributeType); !ok {
		t.Errorf("convertDataType(HashedString) = %T, want *domainmodel.HashedStringAttributeType", got)
	}
	// Control: a String is still a String.
	if _, ok := convertDataType(ast.DataType{Kind: ast.TypeString}).(*domainmodel.StringAttributeType); !ok {
		t.Error("convertDataType(String) no longer builds a StringAttributeType")
	}
}

func TestCreateEntity_HashedStringReachesBackend(t *testing.T) {
	mod := mkModule("M")
	dm := mkDomainModel(mod.ID)

	var created *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created = e
			return nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execCreateEntity(ctx, &ast.CreateEntityStmt{
		Name: ast.QualifiedName{Module: "M", Name: "Cred"},
		Kind: ast.EntityPersistent,
		Attributes: []ast.Attribute{
			{Name: "Pwd", Type: ast.DataType{Kind: ast.TypeHashedString}},
		},
	})
	assertNoError(t, err)
	if created == nil {
		t.Fatal("expected CreateEntity to be called")
	}
	if _, ok := created.Attributes[0].Type.(*domainmodel.HashedStringAttributeType); !ok {
		t.Errorf("Pwd stored as %T, want *domainmodel.HashedStringAttributeType", created.Attributes[0].Type)
	}
}

// Describe must print HashedString, and what it prints must re-parse into the
// same kind — otherwise describe -> exec rewrites the column as a String.
func TestDescribeEntity_HashedStringRoundTrips(t *testing.T) {
	mod := mkModule("M")
	entity := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		Name:        "Cred",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{{
			BaseElement: model.BaseElement{ID: nextID("attr")},
			Name:        "Pwd",
			Type:        &domainmodel.HashedStringAttributeType{},
		}},
	}
	dm := mkDomainModel(mod.ID, entity)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeEntity(ctx, ast.QualifiedName{Module: "M", Name: "Cred"}))

	out := buf.String()
	if !strings.Contains(out, "Pwd: HashedString") {
		t.Errorf("describe does not print `Pwd: HashedString`:\n%s", out)
	}
	stmt := findStmt[*ast.CreateEntityStmt](t, reparse(t, out), out)
	if got := stmt.Attributes[0].Type.Kind; got != ast.TypeHashedString {
		t.Errorf("describe output re-parses Pwd as %v, want HashedString\n%s", got, out)
	}
}

// diff-local renders attributes from raw BSON by substring on $Type, and
// "HashedStringAttributeType" contains "StringAttributeType" — so a hashed
// attribute diffed as a String unless its case is tested first.
func TestDiffLocal_HashedStringAttribute(t *testing.T) {
	got := attributeBsonToMDL(nil, map[string]any{
		"Name":    "Pwd",
		"NewType": map[string]any{"$Type": "DomainModels$HashedStringAttributeType"},
	})
	if !strings.HasPrefix(got, "Pwd: HashedString") {
		t.Errorf("attributeBsonToMDL = %q, want it to start with `Pwd: HashedString`", got)
	}
	// Control: a String attribute still renders as one.
	got = attributeBsonToMDL(nil, map[string]any{
		"Name":    "Name",
		"NewType": map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(100)},
	})
	if !strings.HasPrefix(got, "Name: String(100)") {
		t.Errorf("attributeBsonToMDL = %q, want it to start with `Name: String(100)`", got)
	}
}
