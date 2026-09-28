// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// r3Pairs are R3's old spellings and their canonical forms (ako/mxcli#751): an
// `alter` sets properties in create's `( Key: value, … )` list, a clause takes
// no colon and an attribute definition always has one. Both spellings must
// parse and build the same statements; only the old one may record the code,
// and it must carry the rewrite `fmt --upgrade` applies.
var r3Pairs = []r8Pair{
	// alter settings, every section, and configurations.
	{"settings runtime", "alter settings runtime AfterStartupMicroflow = 'M.Startup', BcryptCost = 11, UseOQLVersion2 = true;",
		"alter settings runtime ( AfterStartupMicroflow: 'M.Startup', BcryptCost: 11, UseOQLVersion2: true );", deprecation.SettingsAssignment},
	{"settings runtime qualified name", "alter settings runtime AfterStartupMicroflow = M.Startup;",
		"alter settings runtime ( AfterStartupMicroflow: M.Startup );", deprecation.SettingsAssignment},
	{"settings language", "alter settings language DefaultLanguageCode = 'en_US';",
		"alter settings language ( DefaultLanguageCode: 'en_US' );", deprecation.SettingsAssignment},
	{"settings workflows", "alter settings workflows UserEntity = 'System.User', DefaultTaskParallelism = 3;",
		"alter settings workflows ( UserEntity: 'System.User', DefaultTaskParallelism: 3 );", deprecation.SettingsAssignment},
	{"settings configuration", "alter settings configuration 'Default' DatabaseType = 'POSTGRESQL', HttpPortNumber = 8080;",
		"alter settings configuration 'Default' ( DatabaseType: 'POSTGRESQL', HttpPortNumber: 8080 );", deprecation.SettingsAssignment},
	{"create configuration", "create or modify configuration 'Acc' DatabaseType = 'HSQLDB', HttpPortNumber = 8081;",
		"create or modify configuration 'Acc' ( DatabaseType: 'HSQLDB', HttpPortNumber: 8081 );", deprecation.SettingsAssignment},

	// alter odata service / client.
	{"odata client", "alter consumed odata service M.Crm set Version = '2.0', MetadataUrl = 'https://x.org/$metadata';",
		"alter consumed odata service M.Crm set ( Version: '2.0', MetadataUrl: 'https://x.org/$metadata' );", deprecation.ODataAlterAssignment},
	{"odata client expression", "alter consumed odata service M.Crm set HttpUsername = 'Bearer ' + @M.Token;",
		"alter consumed odata service M.Crm set ( HttpUsername: 'Bearer ' + @M.Token );", deprecation.ODataAlterAssignment},
	{"odata service", "alter published odata service M.Api set Version = '2.0.0', Summary = 'Orders';",
		"alter published odata service M.Api set ( Version: '2.0.0', Summary: 'Orders' );", deprecation.ODataAlterAssignment},

	// alter styling.
	{"styling equals", "alter styling on page M.P widget ctn1 set Class = 'card', Style = 'margin: 0;', 'Spacing top' = 'Large', 'Full width' = on;",
		"alter styling on page M.P widget ctn1 set ( Class: 'card', Style: 'margin: 0;', 'Spacing top': 'Large', 'Full width': on );", deprecation.StylingAssignment},
	{"styling colon unparenthesised", "alter styling on snippet M.S widget ctn1 set 'Full width': off;",
		"alter styling on snippet M.S widget ctn1 set ( 'Full width': off );", deprecation.StylingAssignment},
	{"styling parenthesised equals", "alter styling on page M.P widget ctn1 set ( Class = 'card' ) clear design properties;",
		"alter styling on page M.P widget ctn1 set ( Class: 'card' ) clear design properties;", deprecation.StylingAssignment},

	// alter entity.
	{"allow create change locally", "alter entity M.Remote set allow_create_change_locally = false;",
		"alter entity M.Remote set ( AllowCreateChangeLocally: false );", deprecation.AllowCreateChangeLocally},
	{"allow create change locally camel", "alter entity M.Remote set AllowCreateChangeLocally = true;",
		"alter entity M.Remote set ( AllowCreateChangeLocally: true );", deprecation.AllowCreateChangeLocally},
	{"modify attribute", "alter entity M.E modify attribute Code String(20) default 'x';",
		"alter entity M.E modify attribute Code: String(20) default 'x';", deprecation.ModifyAttributeColon},
	{"modify attribute", "alter entity M.E modify attribute Amount Decimal;",
		"alter entity M.E modify attribute Amount: Decimal;", deprecation.ModifyAttributeColon},

	// association clauses.
	{"association type", "create association M.A_B from M.A to M.B type: Reference owner: Both storage: Table;",
		"create association M.A_B from M.A to M.B type Reference owner Both storage Table;", deprecation.AssociationClauseColon},

	// alter page / snippet / layout (MDL-DEPR101..103, folded into the registry).
	{"page set equals", "alter page M.P { set Caption = 'Save' on btnSave; set (Caption = 'x', ButtonStyle = Success) on b2; };",
		"alter page M.P { set (Caption: 'Save') on btnSave; set (Caption: 'x', ButtonStyle: Success) on b2; };", deprecation.AlterPageSetEquals},
	{"page set unparenthesised", "alter snippet M.S { set Title: 'T'; };",
		"alter snippet M.S { set (Title: 'T'); };", deprecation.AlterPageSetUnparenthesised},
	{"page drop widget", "alter layout M.L { drop widget a, b; };",
		"alter layout M.L { drop a, b; };", deprecation.AlterPageDropWidget},
}

func TestR3OldSpellingsAreAliases(t *testing.T) {
	for _, p := range r3Pairs {
		t.Run(p.name, func(t *testing.T) {
			old := mustBuild(t, p.old)
			canon := mustBuild(t, p.canon)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical %q recorded %v, want none", p.canon, got)
			}
			got := deprecationCodes(old)
			if len(got) == 0 {
				t.Fatalf("old %q recorded nothing, want %s", p.old, p.code)
			}
			for _, d := range old.Deprecations {
				if d.Code != p.code {
					t.Errorf("old %q recorded %v, want only %s", p.old, got, p.code)
				}
				if d.Fix == nil {
					t.Errorf("old %q: %s recorded without a rewrite (%s)", p.old, d.Code, d.NoFix)
				}
			}
			if len(old.Statements) == 0 {
				t.Fatalf("old %q built no statement", p.old)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The canonical list is read, not merely parsed: each form must build the
// values the executor acts on. Guards against a list alternative the visitor
// never looks at, which would pass the pair test above only if both halves
// built nothing — so the values are pinned here.
func TestR3CanonicalListsCarryTheirValues(t *testing.T) {
	settings := mustBuild(t, "alter settings runtime ( BcryptCost: 11, UseOQLVersion2: true );").Statements[0].(*ast.AlterSettingsStmt)
	if settings.Properties["BcryptCost"] != int64(11) || settings.Properties["UseOQLVersion2"] != true {
		t.Errorf("settings properties: %#v", settings.Properties)
	}
	cfg := mustBuild(t, "create configuration 'Acc' ( HttpPortNumber: 8081 );").Statements[0].(*ast.CreateConfigurationStmt)
	if cfg.Properties["HttpPortNumber"] != "8081" {
		t.Errorf("configuration properties: %#v", cfg.Properties)
	}
	client := mustBuild(t, "alter consumed odata service M.Crm set ( Version: '2.0', HttpUsername: 'Bearer ' + @M.Token );").Statements[0].(*ast.AlterODataClientStmt)
	if client.Changes["Version"] != "2.0" || client.Changes["HttpUsername"] != "'Bearer ' + @M.Token" {
		t.Errorf("odata client changes: %#v", client.Changes)
	}
	styling := mustBuild(t, "alter styling on page M.P widget c set ( Class: 'card', 'Full width': on );").Statements[0].(*ast.AlterStylingStmt)
	if len(styling.Assignments) != 2 || styling.Assignments[0].Value != "card" || !styling.Assignments[1].ToggleOn {
		t.Errorf("styling assignments: %#v", styling.Assignments)
	}
	entity := mustBuild(t, "alter entity M.Remote set ( AllowCreateChangeLocally: true );").Statements[0].(*ast.AlterEntityStmt)
	if entity.Operation != ast.AlterEntitySetAllowCreateChangeLocally || !entity.BoolValue {
		t.Errorf("alter entity: %#v", entity)
	}
}

// An expression is refused on an OData property that takes a plain value, in
// the canonical list as in the old spelling.
func TestR3ODataListRefusesExpressionOnPlainProperty(t *testing.T) {
	for _, src := range []string{
		"alter published odata service M.Api set ( Version: '1' + '2' );",
		"alter consumed odata service M.Crm set ( Version: '1' + '2' );",
	} {
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("%q: want an error for an expression on Version", src)
		}
	}
}
