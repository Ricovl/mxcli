// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// Phase 3.6 (ako/mxcli#755): a constant's and a demo user's clauses become
// their property lists. The rewrite composes with the R9 rewrites on the same
// statement (the comment clause and the trailing folder), and the result
// re-parses with no deprecation left.
func TestUpgrade_HeaderPropertyLists(t *testing.T) {
	src := "create constant M.A type String default 'x';\n" +
		"CREATE OR MODIFY CONSTANT M.B TYPE Integer DEFAULT 42 EXPOSED TO CLIENT;\n" +
		"create or modify constant M.C folder 'Cfg'\n  type String(200)\n  default 'it''s'\n  exposed to client;\n" +
		"create constant M.D type Boolean default true comment 'On or off' exposed to client folder 'Flags';\n" +
		"create demo user 'a' password 'Pw1!' (User);\n" +
		"CREATE DEMO USER 'b' PASSWORD 'Pw1!' ENTITY Administration.Account (Administrator, User);\n" +
		"create or modify demo user 'c' password 'Pw1!'\n  entity M.Account\n  (User);\n"
	want := "create constant M.A ( Type: String, DefaultValue: 'x' );\n" +
		"CREATE OR MODIFY CONSTANT M.B ( Type: Integer, DefaultValue: 42, ExposedToClient: TRUE );\n" +
		"create or modify constant M.C folder 'Cfg'\n  ( Type: String(200), DefaultValue: 'it''s', ExposedToClient: true );\n" +
		"/** On or off */\ncreate constant M.D folder 'Flags' ( Type: Boolean, DefaultValue: true, ExposedToClient: true );\n" +
		"create demo user 'a' ( Password: 'Pw1!', UserRoles: (User) );\n" +
		"CREATE DEMO USER 'b' ( Password: 'Pw1!', Entity: Administration.Account, UserRoles: (Administrator, User) );\n" +
		"create or modify demo user 'c' ( Password: 'Pw1!', Entity: M.Account, UserRoles: (User) );\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{deprecation.ConstantClauses: 4, deprecation.DemoUserClauses: 3} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if len(res.Unrewritten) != 0 {
		t.Errorf("Unrewritten = %+v, want none", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
