// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1501: `revoke write (NewAttr) on entity M.E from M.RoleA,
// M.RoleB, M.RoleC` answered "Unchanged entity access: … — nothing to revoke",
// exit 0, while every rule still held NewAttr at ReadWrite.
//
// The rules were granted one role each (`grant … to M.RoleA where […]`, three
// times), so each stores a single-role ModuleRoles list. RevokeEntityMemberAccess
// only touched a rule whose role set EQUALLED the statement's, {A,B,C}, which no
// rule had — 0 rules modified, reported as already revoked. The member entries
// were there all along (the same revoke from one role narrows them), so the
// reporter's "default member access, no stored entry" hypothesis was not it.
//
// A full REVOKE (RemoveEntityAccessRule) has always worked per role. A partial
// one now matches the same way — every rule whose roles are all named — and
// refuses a rule it would have to narrow for a role the statement does not name.
package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// grantRoles stores a `read *, write *` rule on ZzCust for exactly roles,
// constrained by xpath, the shape `grant … read *, write * … to roles where […]`
// writes.
func grantRoles(t *testing.T, b *Backend, dmID model.ID, xpath string, roles ...string) {
	t.Helper()
	p := grantParams(dmID, "ReadWrite", xpath, nil)
	p.RoleNames = roles
	if err := b.AddEntityAccessRule(p); err != nil {
		t.Fatalf("AddEntityAccessRule %v: %v", roles, err)
	}
}

// ruleRights returns the member rights of the stored rule whose roles are exactly roles.
func ruleRights(t *testing.T, b *Backend, modID model.ID, roles ...string) map[string]string {
	t.Helper()
	for _, r := range rulesOf(t, b, modID) {
		if sameStringSet(r.ModuleRoleNames, roles) {
			return rightsOf(r)
		}
	}
	t.Fatalf("no rule for roles %v", roles)
	return nil
}

func revokeWritePhone() types.EntityAccessRevocation {
	return types.EntityAccessRevocation{RevokeWriteMembers: []string{"MyFirstModule.ZzCust.Phone"}}
}

const ownerXPath = "[MyFirstModule.ZzCust_Owner = '[%CurrentUser%]']"

// The reported shape: one rule per role, one revoke naming all three.
func TestRevokeMember_NarrowsEachSingleRoleRuleTheStatementNames(t *testing.T) {
	b, mod, dm := grantFixture(t)
	roles := []string{"MyFirstModule.RoleA", "MyFirstModule.RoleB", "MyFirstModule.RoleC"}
	for _, r := range roles {
		grantRoles(t, b, dm.ID, ownerXPath, r)
	}

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", roles, revokeWritePhone())
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 3 {
		t.Errorf("rules modified = %d, want 3 — 0 is the reported \"nothing to revoke\"", n)
	}
	for _, r := range roles {
		got := ruleRights(t, b, mod.ID, r)
		if got["Phone"] != "ReadOnly" {
			t.Errorf("%s: Phone = %q, want ReadOnly", r, got["Phone"])
		}
		if got["Name"] != "ReadWrite" || got["Email"] != "ReadWrite" {
			t.Errorf("%s: unnamed members changed: %v", r, got)
		}
	}
}

// Control: a rule naming exactly the statement's roles still matches.
func TestRevokeMember_ExactRoleSetStillMatches(t *testing.T) {
	b, mod, dm := grantFixture(t)
	roles := []string{"MyFirstModule.RoleA", "MyFirstModule.RoleB"}
	grantRoles(t, b, dm.ID, "", roles...)

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", roles, revokeWritePhone())
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 1 {
		t.Errorf("rules modified = %d, want 1", n)
	}
	if got := ruleRights(t, b, mod.ID, roles...); got["Phone"] != "ReadOnly" {
		t.Errorf("Phone = %q, want ReadOnly", got["Phone"])
	}
}

// A rule of other roles only is not the statement's to narrow.
func TestRevokeMember_LeavesRulesOfUnnamedRoles(t *testing.T) {
	b, mod, dm := grantFixture(t)
	grantRoles(t, b, dm.ID, "", "MyFirstModule.RoleA")
	grantRoles(t, b, dm.ID, ownerXPath, "MyFirstModule.RoleD")

	if _, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", []string{"MyFirstModule.RoleA"}, revokeWritePhone()); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := ruleRights(t, b, mod.ID, "MyFirstModule.RoleD"); got["Phone"] != "ReadWrite" {
		t.Errorf("RoleD: Phone = %q, want ReadWrite (not named by the revoke)", got["Phone"])
	}
}

// A rule shared with a role the statement does not name cannot be narrowed
// without narrowing that role too. Refused, naming the rule, and nothing is
// written — not even the rules that could have been narrowed.
func TestRevokeMember_RefusesARuleSharedWithAnUnnamedRole(t *testing.T) {
	b, mod, dm := grantFixture(t)
	grantRoles(t, b, dm.ID, ownerXPath, "MyFirstModule.RoleA")
	grantRoles(t, b, dm.ID, "", "MyFirstModule.RoleA", "MyFirstModule.RoleD")

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", []string{"MyFirstModule.RoleA"}, revokeWritePhone())
	if err == nil {
		t.Fatalf("revoke succeeded (%d rules); want a refusal naming the shared rule", n)
	}
	if msg := err.Error(); !strings.Contains(msg, "MyFirstModule.RoleD") {
		t.Errorf("error does not name the unnamed role: %v", err)
	}
	if got := ruleRights(t, b, mod.ID, "MyFirstModule.RoleA"); got["Phone"] != "ReadWrite" {
		t.Errorf("RoleA-only rule was narrowed before the refusal: Phone = %q", got["Phone"])
	}
	if got := ruleRights(t, b, mod.ID, "MyFirstModule.RoleA", "MyFirstModule.RoleD"); got["Phone"] != "ReadWrite" {
		t.Errorf("shared rule was narrowed: Phone = %q", got["Phone"])
	}
}
