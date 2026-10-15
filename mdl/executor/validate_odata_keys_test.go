// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1426: a published OData entity with no KEY, or with a single
// KEY on an attribute that has no unique validation rule, passed `check` and
// `exec`, and only `mx check` said
//
//	[CE6585] "Published entity 'ModA.EntA' must have a key defined."
//	[CE6624] "Add a unique validation rule to attribute 'Name' of entity
//	         'ModA.EntA' to be able to use it as the key."
//
// The cases below are the measured ones (mx 11.6.6) — see validate_odata_keys.go.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

const keyTestEntities = `
create persistent entity K.Uniq ( Name: string(200) unique error message 'u', Code: string(50) );
create persistent entity K.Plain ( Name: string(200), Code: string(50) );
create persistent entity K.NotNull ( Name: string(200) not null error message 'n', Code: string(50) );
create persistent entity K.Indexed ( Name: string(200), Code: string(50) ) index (Name);
create non-persistent entity K.Np ( Name: string(200), Code: string(50) );
create view entity K.PlainView ( Name: String(200), Code: String(50) ) as (
  select p.Name as Name, p.Code as Code from K.Plain as p
);
`

func keyService(version, entity, expose string) string {
	return `create published odata service K.Svc ( path: 'odata/k/', version: '1.0.0', ODataVersion: ` +
		version + `, namespace: 'K.Ns' )
{ publish entity ` + entity + ` as 'Things' (ReadMode: source) ` + expose + `; };`
}

func keyRuleViolations(t *testing.T, script, projectPath string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("script does not parse: %v\n%s", errs, script)
	}
	var out []linter.Violation
	for _, v := range ValidateODataKeys(prog, projectPath) {
		if v.RuleID == "MDL-ODATA08" || v.RuleID == "MDL-ODATA09" {
			out = append(out, v)
		}
	}
	return out
}

// TestODataKeyRules_Script covers every measured case with the entity defined
// in the same script, so no project is needed.
func TestODataKeyRules_Script(t *testing.T) {
	cases := []struct {
		name     string
		version  string
		entity   string
		expose   string
		wantRule string // "" means the build is clean, so check must be too
		wantCE   string
	}{
		// The controls: what mxbuild accepts.
		{"single key, unique rule (control)", "OData4", "K.Uniq", "expose (Name (KEY), Code)", "", ""},
		{"single key, unique rule, OData3 (control)", "OData3", "K.Uniq", "expose (Name (KEY), Code)", "", ""},
		{"composite key, no unique rule", "OData4", "K.Plain", "expose (Name (KEY), Code (KEY))", "", ""},
		{"single key on a non-persistent entity", "OData4", "K.Np", "expose (Name (KEY), Code)", "", ""},
		{"single key on a view entity", "OData4", "K.PlainView", "expose (Name (KEY), Code)", "", ""},

		// CE6585 — any entity kind.
		{"no key", "OData4", "K.Uniq", "expose (Name, Code)", "MDL-ODATA08", "CE6585"},
		{"no expose clause", "OData4", "K.Uniq", "", "MDL-ODATA08", "CE6585"},
		{"expose (*)", "OData4", "K.Uniq", "expose (*)", "MDL-ODATA08", "CE6585"},
		{"no key, non-persistent", "OData4", "K.Np", "expose (Name, Code)", "MDL-ODATA08", "CE6585"},
		{"no key, view entity", "OData4", "K.PlainView", "expose (Name, Code)", "MDL-ODATA08", "CE6585"},

		// CE6624 — single key, persistable entity, no unique validation rule.
		// Neither the OData version, NOT NULL, nor an index changes it.
		{"single key, no unique rule", "OData4", "K.Plain", "expose (Name (KEY), Code)", "MDL-ODATA09", "CE6624"},
		{"single key, no unique rule, OData3", "OData3", "K.Plain", "expose (Name (KEY), Code)", "MDL-ODATA09", "CE6624"},
		{"single key, not null only", "OData4", "K.NotNull", "expose (Name (KEY), Code)", "MDL-ODATA09", "CE6624"},
		{"single key, index only", "OData4", "K.Indexed", "expose (Name (KEY), Code)", "MDL-ODATA09", "CE6624"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keyRuleViolations(t, keyTestEntities+keyService(tc.version, tc.entity, tc.expose), "")
			if tc.wantRule == "" {
				if len(got) != 0 {
					t.Fatalf("mxbuild accepts this, but check reports: %s", got[0].Message)
				}
				return
			}
			if len(got) != 1 || got[0].RuleID != tc.wantRule {
				t.Fatalf("want exactly one %s, got %v", tc.wantRule, got)
			}
			if got[0].Severity != linter.SeverityError {
				t.Errorf("%s is a build error, so it must refuse; severity is %v", tc.wantRule, got[0].Severity)
			}
			if !strings.Contains(got[0].Message, tc.wantCE) {
				t.Errorf("message does not name %s: %s", tc.wantCE, got[0].Message)
			}
		})
	}
}

// TestODataKeyRules_ProjectEntity is the issue's shape: the entity already
// exists in the project, and the script only publishes it.
func TestODataKeyRules_ProjectEntity(t *testing.T) {
	p := projectFixture(t)
	execAgainst(t, p, `create module K;
create persistent entity K.Uniq ( Name: string(200) unique error message 'u', Code: string(50) );
create persistent entity K.Plain ( Name: string(200), Code: string(50) );
create non-persistent entity K.Np ( Name: string(200), Code: string(50) );`)

	got := keyRuleViolations(t, keyService("OData4", "K.Plain", "expose (Name (KEY), Code)"), p)
	if len(got) != 1 || got[0].RuleID != "MDL-ODATA09" {
		t.Fatalf("key on a stored attribute without a unique rule: want MDL-ODATA09, got %v", got)
	}
	if !strings.Contains(got[0].Message, "K.Plain") || !strings.Contains(got[0].Message, "Name") {
		t.Errorf("message should name the entity and attribute: %s", got[0].Message)
	}

	// The control: the same service over the stored entity WITH the rule.
	if got := keyRuleViolations(t, keyService("OData4", "K.Uniq", "expose (Name (KEY), Code)"), p); len(got) != 0 {
		t.Fatalf("stored unique rule not seen: %v", got)
	}
	if got := keyRuleViolations(t, keyService("OData4", "K.Np", "expose (Name (KEY), Code)"), p); len(got) != 0 {
		t.Fatalf("non-persistent stored entity needs no unique rule: %v", got)
	}

	// The script's definition wins over the stored one.
	script := `create or modify persistent entity K.Plain ( Name: string(200) unique error message 'u', Code: string(50) );
` + keyService("OData4", "K.Plain", "expose (Name (KEY), Code)")
	if got := keyRuleViolations(t, script, p); len(got) != 0 {
		t.Fatalf("script adds the unique rule, yet check reports: %v", got)
	}
}

// TestODataKeyRules_UnknownEntityIsSilent pins the fail-open half: an entity
// neither the script nor a project defines cannot be judged, so CE6624 says
// nothing — while CE6585 needs no entity at all.
func TestODataKeyRules_UnknownEntityIsSilent(t *testing.T) {
	if got := keyRuleViolations(t, keyService("OData4", "K.Elsewhere", "expose (Name (KEY), Code)"), ""); len(got) != 0 {
		t.Fatalf("an unresolvable entity was judged: %v", got)
	}
	got := keyRuleViolations(t, keyService("OData4", "K.Elsewhere", "expose (Name, Code)"), "")
	if len(got) != 1 || got[0].RuleID != "MDL-ODATA08" {
		t.Fatalf("no key needs no entity facts: want MDL-ODATA08, got %v", got)
	}
}

// TestODataKeyRules_InValidateProgram proves the rules are on the path `exec`
// runs as its preflight, not only on `check`'s.
func TestODataKeyRules_InValidateProgram(t *testing.T) {
	prog, errs := visitor.Build(keyTestEntities + keyService("OData4", "K.Plain", "expose (Name (KEY), Code)"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == "MDL-ODATA09" && v.Severity == linter.SeverityError {
			return
		}
	}
	t.Fatal("ValidateProgram (shared by check and exec) does not report MDL-ODATA09")
}
