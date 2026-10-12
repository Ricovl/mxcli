// SPDX-License-Identifier: Apache-2.0

// The System module's access rules are not in the .mpr — mxbuild writes them
// into deployment/model/model.mdp. systemReadAccess is a measurement of that
// file; TestSystemReadAccessMatchesMDP re-checks it against a build given with
// -mdp (the same flag the string-length measurement uses):
//
//	go test ./modelsdk/meta/ -run SystemReadAccess -mdp /path/to/deployment/model/model.mdp
//
// The build must be at security level Production: below it mxbuild has no
// entity access to write.
package meta

import (
	"encoding/binary"
	"os"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Every measured row names an entity SystemEntities knows, so a typo cannot
// make an entity silently "unmeasured".
func TestSystemReadAccessNamesKnownEntities(t *testing.T) {
	for qn := range systemReadAccess {
		name, ok := strings.CutPrefix(qn, "System.")
		if !ok {
			t.Errorf("%s: not a System entity", qn)
			continue
		}
		if _, ok := systemEntityByName(name); !ok {
			t.Errorf("%s is in systemReadAccess but not in SystemEntities", qn)
		}
	}
}

// The rows a page-level lint rule leans on, pinned so an edit to the table is
// a deliberate one. System.User being row-scoped for Administrator too is the
// measured surprise: an Administrator's user list comes from a specialization.
func TestSystemReadAccessKeyRows(t *testing.T) {
	cases := []struct {
		entity string
		want   []SystemReadGrant
	}{
		{"System.User", []SystemReadGrant{{SystemRoleUser, true}, {SystemRoleAdministrator, true}}},
		{"System.UserRole", []SystemReadGrant{{SystemRoleUser, true}, {SystemRoleAdministrator, true}}},
		{"System.Language", []SystemReadGrant{{SystemRoleUser, false}, {SystemRoleAdministrator, false}}},
		{"System.TimeZone", []SystemReadGrant{{SystemRoleUser, false}, {SystemRoleAdministrator, false}}},
		{"System.Session", []SystemReadGrant{{SystemRoleAdministrator, false}}},
		{"System.WorkflowUserTask", []SystemReadGrant{{SystemRoleAdministrator, false}, {SystemRoleUser, true}}},
		{"System.Workflow", []SystemReadGrant{{SystemRoleAdministrator, false}, {SystemRoleUser, true}}},
		{"System.FileDocument", []SystemReadGrant{}},
		{"System.Image", []SystemReadGrant{}},
	}
	for _, c := range cases {
		got, ok := SystemReadAccess(c.entity)
		if !ok {
			t.Errorf("%s: not measured", c.entity)
			continue
		}
		if grantKey(got) != grantKey(c.want) {
			t.Errorf("%s: got %s, want %s", c.entity, grantKey(got), grantKey(c.want))
		}
	}
	if _, ok := SystemReadAccess("System.NoSuchEntity"); ok {
		t.Error("an unknown entity must read as unmeasured, not as 'nobody may read it'")
	}
}

func TestSystemReadAccessMatchesMDP(t *testing.T) {
	if *mdpPath == "" {
		t.Skip("no -mdp given; this test re-measures from a built deployment/model/model.mdp")
	}
	raw, err := os.ReadFile(*mdpPath)
	if err != nil {
		t.Fatalf("read %s: %v", *mdpPath, err)
	}
	measured, err := systemReadAccessFromMDP(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(measured) == 0 {
		t.Fatalf("%s carries no System entity access — is it a Production-level deploy build?", *mdpPath)
	}
	for qn, got := range measured {
		want, ok := systemReadAccess[qn]
		if !ok {
			t.Logf("%s: measured %s, not in the table (add it if pages bind it)", qn, grantKey(got))
			continue
		}
		if grantKey(got) != grantKey(want) {
			t.Errorf("%s: table says %s, %s says %s", qn, grantKey(want), *mdpPath, grantKey(got))
		}
	}
	for qn := range systemReadAccess {
		if _, ok := measured[qn]; !ok {
			t.Errorf("%s is in the table but not in %s", qn, *mdpPath)
		}
	}
}

func grantKey(gs []SystemReadGrant) string {
	parts := make([]string, 0, len(gs))
	for _, g := range gs {
		s := g.ModuleRole
		if g.RowScoped {
			s += "[xpath]"
		}
		parts = append(parts, s)
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ", ") + "}"
}

// systemReadAccessFromMDP maps each System entity in a deployed model to its
// read-granting rules, translating the stored user roles to System module
// roles: a rule for every user role is {System.User, System.Administrator},
// one for exactly the IsSystemAdministrator user roles is
// {System.Administrator}, and one for exactly the others is {System.User}.
func systemReadAccessFromMDP(raw []byte) (map[string][]SystemReadGrant, error) {
	var docs []bson.M
	for off := 0; off+4 <= len(raw); {
		size := int(binary.LittleEndian.Uint32(raw[off : off+4]))
		if size < 5 || off+size > len(raw) {
			break
		}
		var doc bson.M
		if err := bson.Unmarshal(raw[off:off+size], &doc); err == nil {
			docs = append(docs, doc)
		}
		off += size
	}

	admins, users := map[string]bool{}, map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.M:
			if x["$Type"] == "Security$UserRole" {
				name, _ := x["Name"].(string)
				if isAdmin, _ := x["IsSystemAdministrator"].(bool); isAdmin {
					admins[name] = true
				} else {
					users[name] = true
				}
			}
			for _, val := range x {
				walk(val)
			}
		case bson.A:
			for _, val := range x {
				walk(val)
			}
		}
	}
	for _, d := range docs {
		walk(d)
	}

	sameSet := func(got []string, want map[string]bool) bool {
		if len(got) != len(want) {
			return false
		}
		for _, g := range got {
			if !want[g] {
				return false
			}
		}
		return true
	}
	all := map[string]bool{}
	for r := range admins {
		all[r] = true
	}
	for r := range users {
		all[r] = true
	}

	out := map[string][]SystemReadGrant{}
	for _, d := range docs {
		if d["$Type"] != "DomainModels$DomainModel" {
			continue
		}
		entities, _ := d["Entities"].(bson.A)
		for _, ev := range entities {
			e, _ := ev.(bson.M)
			qn, _ := e["QualifiedName"].(string)
			if !strings.HasPrefix(qn, "System.") {
				continue
			}
			grants := []SystemReadGrant{}
			rules, _ := e["AccessRules"].(bson.A)
			for _, rv := range rules {
				rule, _ := rv.(bson.M)
				if rule == nil || !ruleGrantsRead(rule) {
					continue
				}
				var roles []string
				if ar, ok := rule["AllowedUserRoles"].(bson.A); ok {
					for _, r := range ar {
						if s, ok := r.(string); ok {
							roles = append(roles, s)
						}
					}
				}
				xpath, _ := rule["XPathConstraint"].(string)
				scoped := strings.TrimSpace(xpath) != ""
				switch {
				case sameSet(roles, all):
					grants = append(grants, SystemReadGrant{SystemRoleUser, scoped}, SystemReadGrant{SystemRoleAdministrator, scoped})
				case sameSet(roles, admins):
					grants = append(grants, SystemReadGrant{SystemRoleAdministrator, scoped})
				case sameSet(roles, users):
					grants = append(grants, SystemReadGrant{SystemRoleUser, scoped})
				default:
					grants = append(grants, SystemReadGrant{"?" + strings.Join(roles, "+"), scoped})
				}
			}
			out[qn] = grants
		}
	}
	return out, nil
}

func ruleGrantsRead(rule bson.M) bool {
	members, _ := rule["MemberAccesses"].(bson.A)
	for _, mv := range members {
		m, _ := mv.(bson.M)
		switch m["AccessRights"] {
		case "ReadOnly", "ReadWrite":
			return true
		}
	}
	return false
}
