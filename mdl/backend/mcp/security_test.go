// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func TestBuildAccessRuleValue(t *testing.T) {
	v := buildAccessRuleValue(backend.EntityAccessRuleParams{
		EntityName:          "Expense",
		RoleNames:           []string{"ExpenseApproval.Manager"},
		AllowCreate:         true,
		AllowDelete:         true,
		DefaultMemberAccess: "ReadWrite",
		MemberAccesses: []types.EntityMemberAccess{
			{AttributeRef: "ExpenseApproval.Expense.Title", AccessRights: "ReadWrite"},
			{AssociationRef: "ExpenseApproval.Expense_Employee", AccessRights: "ReadOnly"},
		},
	})

	if v["$Type"] != "DomainModels$AccessRule" {
		t.Fatalf("$Type = %v", v["$Type"])
	}
	if v["defaultMemberAccessRights"] != "ReadWrite" || v["allowCreate"] != true || v["allowDelete"] != true {
		t.Fatalf("rule leaves = %#v", v)
	}
	if _, hasXPath := v["xPathConstraint"]; hasXPath {
		t.Fatal("empty xPathConstraint should be omitted")
	}
	mas := v["memberAccesses"].([]any)
	if len(mas) != 2 {
		t.Fatalf("memberAccesses len = %d", len(mas))
	}
	attr := mas[0].(map[string]any)
	if attr["attribute"] != "ExpenseApproval.Expense.Title" || attr["accessRights"] != "ReadWrite" {
		t.Fatalf("attr member = %#v", attr)
	}
	if _, hasAssoc := attr["association"]; hasAssoc {
		t.Fatal("empty association ref must be omitted (PED rejects empty references)")
	}
	assoc := mas[1].(map[string]any)
	if assoc["association"] != "ExpenseApproval.Expense_Employee" {
		t.Fatalf("assoc member = %#v", assoc)
	}
	if _, hasAttr := assoc["attribute"]; hasAttr {
		t.Fatal("empty attribute ref must be omitted")
	}
}

func TestBuildAccessRuleValue_Defaults(t *testing.T) {
	v := buildAccessRuleValue(backend.EntityAccessRuleParams{RoleNames: []string{"M.R"}})
	if v["defaultMemberAccessRights"] != "None" {
		t.Fatalf("default rights should fall back to None, got %v", v["defaultMemberAccessRights"])
	}
	if _, has := v["memberAccesses"]; has {
		t.Fatal("no member accesses -> key omitted")
	}
}

func TestRoleSetsEqual(t *testing.T) {
	if !roleSetsEqual(normalizeRoleSet([]string{"A.x", "B.y"}), normalizeRoleSet([]string{"B.y", "A.x"})) {
		t.Fatal("order-independent equality failed")
	}
	if roleSetsEqual(normalizeRoleSet([]string{"A.x"}), normalizeRoleSet([]string{"A.x", "B.y"})) {
		t.Fatal("different sizes must not be equal")
	}
}

// TestLive_EntityAccessRuleReject exercises the live entity-access read + dedup
// path: granting a role set that already has a rule on an entity must hit the
// "already exists" rejection, with no second write (PED is add-only for access
// rules, so a duplicate could never be removed). Skipped unless MXCLI_MCP_URL is
// set.
//
// It builds its own fixture in MXCLI_MCP_MODULE (#924): a fresh entity
// Zz_R12_AccessFixture_<stamp>, plus one rule for MXCLI_MCP_ROLE (default
// <module>.User). Module roles cannot be authored over MCP, so when that role does
// not exist the test skips and says which role to set. The fixture is left in
// place — PED cannot remove the rule, and keeping the entity makes a run
// inspectable.
//
//	MXCLI_MCP_URL=http://localhost/mcp MXCLI_MCP_DIAL=host.docker.internal:7793 \
//	MXCLI_MCP_MODULE=MyFirstModule MXCLI_MCP_ROLE=MyFirstModule.User \
//	go test ./mdl/backend/mcp/ -run TestLive_EntityAccessRuleReject -v
func TestLive_EntityAccessRuleReject(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := envOr("MXCLI_MCP_MODULE", "MyFirstModule")
	role := envOr("MXCLI_MCP_ROLE", module+".User")

	c, err := NewClient(ClientOptions{URL: url, Dial: os.Getenv("MXCLI_MCP_DIAL")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	b := &Backend{client: c}
	unit := model.ID(sessionDMPrefix + module)

	// Fixture: a fresh entity with one attribute, and one rule for the role.
	entity := "Zz_R12_AccessFixture_" + time.Now().Format("0102150405")
	if err := b.CreateEntity(unit, newPersistentEntity(entity,
		attr("Title", &domainmodel.StringAttributeType{Length: 200}))); err != nil {
		t.Fatalf("create fixture entity %s.%s: %v", module, entity, err)
	}
	grant := backend.EntityAccessRuleParams{
		UnitID:              unit,
		EntityName:          entity,
		RoleNames:           []string{role},
		DefaultMemberAccess: "ReadOnly",
		MemberAccesses: []types.EntityMemberAccess{
			{AttributeRef: module + "." + entity + ".Title", AccessRights: "ReadOnly"},
		},
	}
	if err := b.AddEntityAccessRule(grant); err != nil {
		t.Skipf("cannot grant %s on the fixture entity %s.%s (module roles cannot be created over MCP; "+
			"set MXCLI_MCP_ROLE to an existing role of %s): %v", role, module, entity, module, err)
	}
	t.Logf("fixture %s.%s has a rule for %s (left in place)", module, entity, role)

	idx, err := b.entityIndex(module, entity)
	if err != nil {
		t.Fatalf("entityIndex: %v", err)
	}
	sets, err := b.entityAccessRuleRoleSets(module, idx)
	if err != nil {
		t.Fatalf("entityAccessRuleRoleSets: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("fixture has %d access rule(s), want 1", len(sets))
	}

	// Grant the same role again -> must be rejected, with no write.
	err = b.AddEntityAccessRule(grant)
	if err == nil {
		t.Fatal("expected AddEntityAccessRule to reject an existing role set, got nil (a duplicate rule may have been written and PED cannot remove it)")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an 'already exists' rejection, got: %v", err)
	}
	if sets, err := b.entityAccessRuleRoleSets(module, idx); err != nil || len(sets) != 1 {
		t.Fatalf("after the rejected grant: %d rule(s), err %v; want 1", len(sets), err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
