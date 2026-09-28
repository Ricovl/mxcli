// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R10 (ako/mxcli#755): `alter app security` sets properties, in create's
// ( Key: value, … ) list, with the Security$ProjectSecurity property names.
// Each clause form is a deprecated alias building the same statement.
func TestAlterAppSecurityClausesAreAliasesOfTheList(t *testing.T) {
	cases := []struct{ old, canonical string }{
		{"alter app security level production;", "alter app security ( SecurityLevel: production );"},
		{"ALTER APP SECURITY LEVEL PROTOTYPE;", "ALTER APP SECURITY ( SecurityLevel: prototype );"},
		{"alter app security level off;", "alter app security ( SecurityLevel: off );"},
		{"alter app security demo users on;", "alter app security ( EnableDemoUsers: true );"},
		{"alter app security demo users off;", "alter app security ( EnableDemoUsers: false );"},
		{"alter app security guest access on role Guest;",
			"alter app security ( EnableGuestAccess: true, GuestUserRole: Guest );"},
		{"alter app security guest access on;", "alter app security ( EnableGuestAccess: true );"},
		{"alter app security guest access off;", "alter app security ( EnableGuestAccess: false );"},
		{"alter app security strict mode on;", "alter app security ( StrictMode: true );"},
		{"alter app security strict mode off;", "alter app security ( StrictMode: false );"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.AppSecurityClause}) {
				t.Errorf("old recorded %v, want [%s]", got, deprecation.AppSecurityClause)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The list sets several properties in one statement, which no clause form
// could; `project security` with the list is only R10's name alias.
func TestAlterAppSecurityListSetsSeveral(t *testing.T) {
	prog := mustBuild(t, "alter app security ( SecurityLevel: production, EnableDemoUsers: false, "+
		"EnableGuestAccess: true, GuestUserRole: 'Guest', StrictMode: true, );")
	on, off := true, false
	want := &ast.AlterProjectSecurityStmt{SecurityLevel: "Production", DemoUsersEnabled: &off,
		GuestAccessEnabled: &on, GuestUserRole: "Guest", StrictModeEnabled: &on}
	if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], want) {
		t.Errorf("got %#v, want %#v", prog.Statements, want)
	}
	if got := deprecationCodes(mustBuild(t, "alter project security ( StrictMode: true );")); !reflect.DeepEqual(got, []string{deprecation.AppSecurity}) {
		t.Errorf("recorded %v, want only %s", got, deprecation.AppSecurity)
	}
}

// A key or value the list does not take is an error that names what it takes,
// not a silently ignored property.
func TestAlterAppSecurityListRefusesUnknown(t *testing.T) {
	for src, want := range map[string]string{
		"alter app security ( Level: production );":                   "SecurityLevel",
		"alter app security ( SecurityLevel: high );":                 "production, prototype or off",
		"alter app security ( StrictMode: 'yes' );":                   "true or false",
		"alter app security ( GuestUserRole: 12 );":                   "user role",
		"alter app security ( StrictMode: true, StrictMode: false );": "twice",
	} {
		t.Run(src, func(t *testing.T) {
			_, errs := Build(src)
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), want) {
				t.Errorf("errs = %v, want one mentioning %q", errs, want)
			}
		})
	}
}
