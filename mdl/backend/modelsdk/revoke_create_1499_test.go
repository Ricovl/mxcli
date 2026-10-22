// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1499: `revoke create on entity M.E from M.RoleA, M.RoleB,
// M.RoleC` answered "Unchanged entity access: … — nothing to revoke", exit 0,
// while `show access` still listed `Rights: create, read` for all three roles.
//
// Same matcher as #1501, a different right: the rules were granted one role
// each, and the partial revoke only touched a rule whose role set EQUALLED the
// statement's. #1501's tests cover a member right only; the entity-level create
// right is pinned here, in the reported shape (two rules constrained, one not —
// the reporter's control that the XPath is not the cause).
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func TestRevokeCreate_NarrowsEachSingleRoleRuleTheStatementNames(t *testing.T) {
	b, mod, dm := grantFixture(t)
	roles := []string{"MyFirstModule.RoleA", "MyFirstModule.RoleB", "MyFirstModule.RoleC"}
	xpaths := map[string]string{
		"MyFirstModule.RoleA": ownerXPath,
		"MyFirstModule.RoleB": ownerXPath,
		"MyFirstModule.RoleC": "",
	}
	for _, r := range roles {
		// grant create, read * on … to r [where …]
		p := grantParams(dm.ID, "ReadOnly", xpaths[r], nil)
		p.RoleNames = []string{r}
		p.AllowCreate = true
		if err := b.AddEntityAccessRule(p); err != nil {
			t.Fatalf("AddEntityAccessRule %s: %v", r, err)
		}
	}

	n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", roles, types.EntityAccessRevocation{RevokeCreate: true})
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 3 {
		t.Errorf("rules modified = %d, want 3 — 0 is the reported \"nothing to revoke\"", n)
	}
	for _, rule := range rulesOf(t, b, mod.ID) {
		if len(rule.ModuleRoleNames) != 1 {
			t.Fatalf("rule roles = %v, want one role each", rule.ModuleRoleNames)
		}
		r := rule.ModuleRoleNames[0]
		if rule.AllowCreate {
			t.Errorf("%s: create still granted", r)
		}
		if rule.XPathConstraint != xpaths[r] {
			t.Errorf("%s: XPath = %q, want %q", r, rule.XPathConstraint, xpaths[r])
		}
		if string(rule.DefaultMemberAccessRights) != "ReadOnly" {
			t.Errorf("%s: default member access = %q, want ReadOnly (read stays)", r, rule.DefaultMemberAccessRights)
		}
		for m, got := range rightsOf(rule) {
			if got != "ReadOnly" {
				t.Errorf("%s: %s = %q, want ReadOnly (read stays)", r, m, got)
			}
		}
	}

	// A rerun is the honest idempotent case: nothing left to revoke.
	if n, err := b.RevokeEntityMemberAccess(dm.ID, "ZzCust", roles, types.EntityAccessRevocation{RevokeCreate: true}); err != nil || n != 0 {
		t.Errorf("rerun: modified = %d, err = %v; want 0, nil", n, err)
	}
}
