// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1472 item 4: the WHERE of an access rule was never
// resolved. Measured on 11.13.0 at security level Prototype, the reported
// script built to
//
//	CE1613 "The selected association 'Administration.NoSuchAssoc' no longer
//	        exists." at Access rule of entity 'Administration.Account'
//
// while check printed "All references valid". Run on the real path against
// the Studio Pro-authored PedApp fixture, as the other grant checks are.
func TestCheckGrantXPath_UnknownMemberIsRefused(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"the reported shape: an association path that names nothing",
			"grant read * on entity Administration.Account to Administration.User where [Administration.NoSuchAssoc/Administration.Account/NoSuchAttr = 'x'];",
			`association "Administration.NoSuchAssoc"`},
		{"a bare attribute the entity does not have",
			"grant read * on entity Administration.Account to Administration.User where [NoSuchAttr = 'x'];",
			`attribute "NoSuchAttr"`},
	}
	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(agreeCheck(t, exec, dir, "mdl 1;\n"+c.src), "\n")
			if !strings.Contains(got, c.want) || !strings.Contains(got, "access rule") {
				t.Errorf("check --references must report %q on the access rule, reported:\n%s", c.want, got)
			}
		})
	}
}

func TestCheckGrantXPath_ResolvableMembersPass(t *testing.T) {
	cases := []struct{ name, src string }{
		{"own attribute",
			"grant read * on entity Administration.Account to Administration.User where [FullName = 'x'];"},
		{"attribute inherited from System.User",
			"grant read * on entity Administration.Account to Administration.User where [Name = 'x'];"},
		{"implicit member",
			"grant read * on entity Administration.Account to Administration.User where [id = '[%CurrentUser%]'];"},
		{"association inherited from System.User",
			"grant read * on entity Administration.Account to Administration.User where [System.UserRoles = '[%UserRole_User%]'];"},
		{"attribute the script adds",
			"alter entity Administration.Account add attribute Region: String(20);\n" +
				"grant read * on entity Administration.Account to Administration.User where [Region = 'x'];"},
		{"association and entity the script creates",
			"create persistent entity MyFirstModule.Thing (Name: String(10));\n" +
				"create association Administration.Account_Thing from Administration.Account to MyFirstModule.Thing;\n" +
				"grant read * on entity Administration.Account to Administration.User where [Administration.Account_Thing/MyFirstModule.Thing/Name = 'x'];"},
		// Measured on 11.13.0: builds clean (doctype-tests/08 writes this).
		{"member spelled in another case",
			"create persistent entity MyFirstModule.Thing (status: String(10));\n" +
				"grant read * on entity MyFirstModule.Thing to MyFirstModule.User where [Status = 'Open'];"},
		{"stored member spelled in another case",
			"grant read * on entity Administration.Account to Administration.User where [fullname = 'x'];"},
		{"entity the script creates",
			"create persistent entity MyFirstModule.Thing (Name: String(10));\n" +
				"grant read * on entity MyFirstModule.Thing to MyFirstModule.User where [Name = 'x'];"},
	}
	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agreeCheck(t, exec, dir, "mdl 1;\n"+c.src); len(got) != 0 {
				t.Fatalf("a resolvable constraint was refused:\n%s", strings.Join(got, "\n"))
			}
		})
	}
}
