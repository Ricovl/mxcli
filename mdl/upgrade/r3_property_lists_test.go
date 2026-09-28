// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"
)

// R3 (ako/mxcli#751): every old property-list spelling upgrades to exactly
// create's `( Key: value, … )` list, and the colons to where R3 puts them. The
// visitor test (mdl/visitor/r3_property_lists_test.go) proves each pair builds
// the same statements; this one proves the rewrite produces that pair, in the
// layout it was written in.
func TestUpgrade_R3PropertyLists(t *testing.T) {
	cases := []struct{ old, want string }{
		{"alter settings runtime AfterStartupMicroflow = 'M.Startup', BcryptCost = 11;\n",
			"alter settings runtime ( AfterStartupMicroflow: 'M.Startup', BcryptCost: 11 );\n"},
		// describe's old layout: one property per line.
		{"alter settings runtime\n  HashAlgorithm = 'BCrypt',\n  BcryptCost = 11;\n",
			"alter settings runtime (\n  HashAlgorithm: 'BCrypt',\n  BcryptCost: 11\n);\n"},
		{"ALTER SETTINGS MODEL BcryptCost = 11;\n", "ALTER SETTINGS RUNTIME ( BcryptCost: 11 );\n"},
		// Indented: the closing parenthesis lines up with the statement.
		{"  alter settings configuration 'Default'\n    HttpPortNumber = 8080;\n",
			"  alter settings configuration 'Default' (\n    HttpPortNumber: 8080\n  );\n"},
		{"alter settings LANGUAGE\n  DefaultLanguageCode = 'en_US';\n",
			"alter settings LANGUAGE (\n  DefaultLanguageCode: 'en_US'\n);\n"},
		{"alter settings configuration 'Default' HttpPortNumber = 8080;\n",
			"alter settings configuration 'Default' ( HttpPortNumber: 8080 );\n"},
		{"create or modify configuration 'Acc'\n  DatabaseType = 'HSQLDB',\n  HttpPortNumber = 8081;\n",
			"create or modify configuration 'Acc' (\n  DatabaseType: 'HSQLDB',\n  HttpPortNumber: 8081\n);\n"},
		{"alter odata client M.Crm set Version = '2.0', HttpUsername = 'Bearer ' + @M.Token;\n",
			"alter consumed odata service M.Crm set ( Version: '2.0', HttpUsername: 'Bearer ' + @M.Token );\n"},
		{"ALTER PUBLISHED ODATA SERVICE M.Api SET Version='2.0.0';\n",
			"ALTER PUBLISHED ODATA SERVICE M.Api SET ( Version:'2.0.0' );\n"},
		{"alter styling on page M.P widget ctn1 set Class = 'card', 'Full width' = on;\n",
			"alter styling on page M.P widget ctn1 set ( Class: 'card', 'Full width': on );\n"},
		{"alter styling on page M.P widget ctn1 set ( Class = 'card' ) clear design properties;\n",
			"alter styling on page M.P widget ctn1 set ( Class: 'card' ) clear design properties;\n"},
		{"alter entity M.Remote set allow_create_change_locally = true;\n",
			"alter entity M.Remote set ( AllowCreateChangeLocally: true );\n"},
		{"alter entity M.E modify attribute Code String(20), modify column Amount Decimal;\n",
			"alter entity M.E modify attribute Code: String(20), modify column Amount: Decimal;\n"},
		{"create association M.A_B from M.A to M.B type: Reference owner:Both storage :Table;\n",
			"create association M.A_B from M.A to M.B type Reference owner Both storage Table;\n"},
		{"alter page M.P {\n  set Caption = 'Save' on btnSave;\n  set (Caption = 'x', ButtonStyle = Success) on b2;\n  set Title: 'T';\n  drop widget a, b;\n};\n",
			"alter page M.P {\n  set (Caption: 'Save') on btnSave;\n  set (Caption: 'x', ButtonStyle: Success) on b2;\n  set (Title: 'T');\n  drop a, b;\n};\n"},
		// A nested respelling inside the value is rewritten with the list.
		{"alter page M.P { set Action = show_page M.Q(Item: $currentObject) on b; };\n",
			"alter page M.P { set (Action: show page M.Q(Item = $currentObject)) on b; };\n"},
	}
	for _, c := range cases {
		res := mustUpgrade(t, c.old, Options{})
		if res.Source != c.want {
			t.Errorf("upgrade of\n%s got:\n%s want:\n%s", c.old, res.Source, c.want)
		}
		if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
			t.Errorf("upgrade is not idempotent on\n%s", res.Source)
		}
	}
}

// A comment between the tokens a rewrite touches survives it: the rewrite
// removes the old token and the blank space after it, never the text between
// two tokens (upgrade.go: "comments, layout … are kept").
func TestUpgrade_R3KeepsCommentsInsideTheRewrite(t *testing.T) {
	cases := []struct{ old, want string }{
		{"alter page M.P { drop widget -- old\n  a; };\n",
			"alter page M.P { drop -- old\n  a; };\n"},
		{"alter page M.P { drop widget /* x */ a; };\n",
			"alter page M.P { drop /* x */ a; };\n"},
		{"create association M.A_B from M.A to M.B type: /* c */ Reference;\n",
			"create association M.A_B from M.A to M.B type /* c */ Reference;\n"},
		{"create association M.A_B from M.A to M.B type /* c */ : Reference;\n",
			"create association M.A_B from M.A to M.B type /* c */ Reference;\n"},
		{"alter settings runtime BcryptCost /* c */ = 11;\n",
			"alter settings runtime ( BcryptCost /* c */ : 11 );\n"},
		{"alter entity M.Remote set allow_create_change_locally /* c */ = true;\n",
			"alter entity M.Remote set ( AllowCreateChangeLocally /* c */ : true );\n"},
	}
	for _, c := range cases {
		res := mustUpgrade(t, c.old, Options{})
		if res.Source != c.want {
			t.Errorf("upgrade of\n%s got:\n%s want:\n%s", c.old, res.Source, c.want)
		}
		if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
			t.Errorf("upgrade is not idempotent on\n%s", res.Source)
		}
	}
}
