// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#707: a user role's positional module roles become its property
// list, and a REST header written as an expression becomes the value template.
func TestUpgrade_Issue707UserRoleAndRestHeader(t *testing.T) {
	src := "create user role Clerk (M.User, M.Viewer);\n" +
		"CREATE OR MODIFY USER ROLE Admin (M.Admin) MANAGE ALL ROLES;\n" +
		"create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {\n" +
		"  operation Get (Method: get, Path: '/a', Parameters: ($Token: String, $Key: String),\n" +
		"    Headers: ('Authorization' = 'Bearer ' + $Token, 'X-Key' = $Key, 'X-Odd' = '{x' + $Key), Response: none)\n" +
		"};\n"
	want := "create user role Clerk ( ModuleRoles: (M.User, M.Viewer) );\n" +
		"CREATE OR MODIFY USER ROLE Admin ( ModuleRoles: (M.Admin), ManageAllRoles: true );\n" +
		"create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) {\n" +
		"  operation Get (Method: get, Path: '/a', Parameters: ($Token: String, $Key: String),\n" +
		"    Headers: ('Authorization' = 'Bearer {Token}', 'X-Key' = '{Key}', 'X-Odd' = '{x' + $Key), Response: none)\n" +
		"};\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{deprecation.UserRolePositional: 2, deprecation.RestHeaderConcat: 2} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	// A prefix holding a brace would turn into a placeholder: reported, not rewritten.
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.RestHeaderConcat {
		t.Errorf("Unrewritten = %+v, want the '{x' + $Key header", res.Unrewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}
