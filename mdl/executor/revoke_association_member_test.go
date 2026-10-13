// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// revokeAssocFixture is mendixlabs/mxcli#1225's model: ModA.Tag owns the
// Reference association Tag_GuestGroup (Tag -> GuestGroup), so the MemberAccess
// for it lives on Tag's rule, stored as "ModA.Tag_GuestGroup" — qualified by the
// module, not by the entity as an attribute is.
func revokeAssocFixture(t *testing.T) (*mock.MockBackend, *[]types.EntityAccessRevocation) {
	t.Helper()
	mod := mkModule("ModA")
	tag := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Tag", Persistable: true,
		Attributes: []*domainmodel.Attribute{{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: "Label"}},
	}
	group := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "GuestGroup", Persistable: true}
	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{tag, group},
		Associations: []*domainmodel.Association{mkAssociation(mod.ID, "Tag_GuestGroup", tag.ID, group.ID)},
	}
	ms := &security.ModuleSecurity{ContainerID: mod.ID, ModuleRoles: []*security.ModuleRole{{Name: "User"}}}
	var got []types.EntityAccessRevocation
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:    func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:     func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc:   func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetModuleSecurityFunc:  func(model.ID) (*security.ModuleSecurity, error) { return ms, nil },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) { return []*security.ModuleSecurity{ms}, nil },
		RevokeEntityMemberAccessFunc: func(_ model.ID, _ string, _ []string, rev types.EntityAccessRevocation) (int, error) {
			got = append(got, rev)
			return 1, nil
		},
	}
	return mb, &got
}

func revokeWriteOf(entity string, members ...string) *ast.RevokeEntityAccessStmt {
	return &ast.RevokeEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "ModA", Name: entity},
		Roles:  []ast.QualifiedName{{Module: "ModA", Name: "User"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessWriteMembers, Members: members}},
	}
}

// `revoke ModA.User on ModA.Tag (write (Tag_GuestGroup))` qualified the
// association like an attribute, "ModA.Tag.Tag_GuestGroup", which matches no
// stored MemberAccess — the revoke said "nothing to revoke", exited 0, and the
// association stayed ReadWrite (mendixlabs/mxcli#1225).
func TestRevokeWriteAssociationMemberUsesStoredRef(t *testing.T) {
	mb, got := revokeAssocFixture(t)
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertNoError(t, execRevokeEntityAccess(ctx, revokeWriteOf("Tag", "Tag_GuestGroup", "Label")))
	if len(*got) != 1 {
		t.Fatalf("backend revoke called %d times, want 1", len(*got))
	}
	want := map[string]bool{"ModA.Tag_GuestGroup": true, "ModA.Tag.Label": true}
	for _, ref := range (*got)[0].RevokeWriteMembers {
		if !want[ref] {
			t.Errorf("revoked member ref %q matches no stored MemberAccess", ref)
		}
		delete(want, ref)
	}
	for ref := range want {
		t.Errorf("member %q was not revoked", ref)
	}
}

// A member the entity's rule cannot carry is a mistake in the script, not an
// already-revoked right: the TO end of a Default-owner association has no
// MemberAccess for it (CLAUDE.md, ParentPointer = FROM), and a typo has none
// either. Mirrors GRANT, which refuses an unmatched member (#758).
func TestRevokeUnknownMemberIsAnError(t *testing.T) {
	for name, stmt := range map[string]*ast.RevokeEntityAccessStmt{
		"typo":        revokeWriteOf("Tag", "Lable"),
		"TO end":      revokeWriteOf("GuestGroup", "Tag_GuestGroup"),
		"assoc typo":  revokeWriteOf("Tag", "Tag_Guestgroup"),
		"mixed known": revokeWriteOf("Tag", "Label", "Nope"),
	} {
		t.Run(name, func(t *testing.T) {
			mb, got := revokeAssocFixture(t)
			ctx, _ := newMockCtx(t, withBackend(mb))
			err := execRevokeEntityAccess(ctx, stmt)
			if err == nil || !strings.Contains(err.Error(), "has no member") {
				t.Fatalf("err = %v, want a 'has no member' error", err)
			}
			if len(*got) != 0 {
				t.Errorf("rule was modified despite the error")
			}
		})
	}
}
