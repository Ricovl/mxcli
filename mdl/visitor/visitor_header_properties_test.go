// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// Phase 3.6 (ako/mxcli#755): a constant's and a demo user's header is a
// ( Key: value ) list keyed by Studio Pro's property names, as a user role's
// (#707) and a scheduled event's already are. The clause forms are deprecated
// aliases that build the same statement.
func TestHeaderPropertyListsAreCanonical(t *testing.T) {
	cases := []struct{ code, old, canonical string }{
		{deprecation.ConstantClauses,
			"create constant M.Url type String default 'x';",
			"create constant M.Url ( Type: String, DefaultValue: 'x' );"},
		{deprecation.ConstantClauses,
			"create or modify constant M.Url folder 'Config' type String default 'x' exposed to client;",
			"create or modify constant M.Url folder 'Config' ( Type: String, DefaultValue: 'x', ExposedToClient: true, );"},
		{deprecation.ConstantClauses,
			"CREATE CONSTANT M.Max TYPE Integer DEFAULT 42;",
			"CREATE CONSTANT M.Max ( Type: Integer, DefaultValue: 42 );"},
		{deprecation.ConstantClauses,
			"create constant M.On type Boolean default true;",
			"create constant M.On ( ExposedToClient: false, DefaultValue: true, Type: Boolean );"},
		{deprecation.ConstantClauses,
			"create constant M.Name type String(200) default '';",
			"create constant M.Name ( Type: String(200), DefaultValue: '' );"},
		{deprecation.DemoUserClauses,
			"create demo user 'demo' password 'Pw1!' (User);",
			"create demo user 'demo' ( Password: 'Pw1!', UserRoles: (User) );"},
		{deprecation.DemoUserClauses,
			"create or modify demo user 'demo' password 'Pw1!' entity Administration.Account (Administrator, User);",
			"create or modify demo user 'demo' ( Password: 'Pw1!', Entity: Administration.Account, UserRoles: (Administrator, User), );"},
		{deprecation.DemoUserClauses,
			"CREATE DEMO USER 'demo' PASSWORD 'Pw1!' ENTITY M.Account (User);",
			"CREATE DEMO USER 'demo' ( UserRoles: (User), Entity: M.Account, Password: 'Pw1!' );"},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{c.code}) {
				t.Errorf("old form recorded %v, want [%s]", got, c.code)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical form recorded %v, want none", got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The canonical forms build what they say: the property list is not a
// synonym that happens to parse, it carries every value.
func TestHeaderPropertyListsCarryTheirValues(t *testing.T) {
	c := mustBuild(t, "create constant M.Url folder 'Cfg' ( Type: String, DefaultValue: 'https://x', ExposedToClient: true );").
		Statements[0].(*ast.CreateConstantStmt)
	if c.DataType.Kind != ast.TypeString || c.DefaultValue != "https://x" || !c.ExposedToClient || c.Folder != "Cfg" {
		t.Errorf("constant = %+v", c)
	}
	for _, src := range []string{"create demo user 'd' ( Password: 'p' );", "create demo user 'd' ( Password: 'p', UserRoles: () );"} {
		if u := mustBuild(t, src).Statements[0].(*ast.CreateDemoUserStmt); len(u.UserRoles) != 0 || u.Password != "p" {
			t.Errorf("%s built %+v", src, u)
		}
	}
	d := mustBuild(t, "create demo user 'demo' ( Password: 'Pw1!', Entity: M.Account, UserRoles: (Admin, User) );").
		Statements[0].(*ast.CreateDemoUserStmt)
	if d.UserName != "demo" || d.Password != "Pw1!" || d.Entity != "M.Account" || !reflect.DeepEqual(d.UserRoles, []string{"Admin", "User"}) {
		t.Errorf("demo user = %+v", d)
	}
}

// The lists are new syntax, so a key or value they do not take is an error
// under every language version: there is no older reading to keep.
func TestHeaderPropertyListsRefuseWhatTheyDoNotTake(t *testing.T) {
	cases := map[string]string{
		"create constant M.C ( Type: String, DefaultValu: 'x' );":                          "did you mean 'DefaultValue'",
		"create constant M.C ( Type: 'String', DefaultValue: 'x' );":                       "Type takes a data type",
		"create constant M.C ( Type: String, DefaultValue: String );":                      "DefaultValue takes a literal",
		"create constant M.C ( Type: String, DefaultValue: 'x', ExposedToClient: 'yes' );": "ExposedToClient takes true or false",
		"create constant M.C ( DefaultValue: 'x' );":                                       "needs Type",
		"create constant M.C ( Type: String );":                                            "needs DefaultValue",
		"create constant M.C ( Type: String, Type: Integer, DefaultValue: 1 );":            "sets Type twice",
		"create constant M.C ( Type: String, DefaultValue: 'x' ) exposed to client;":       "ExposedToClient: true",
		"create demo user 'd' ( Password: 'p', UserRole: (User) );":                        "did you mean 'UserRoles'",
		"create demo user 'd' ( UserRoles: (User) );":                                      "needs Password",
		"create demo user 'd' ( Password: p, UserRoles: (User) );":                         "Password takes a string",
		"create demo user 'd' ( Password: 'p', Entity: 'M.Account', UserRoles: (User) );":  "Entity takes",
		"create demo user 'd' ( Password: 'p', UserRoles: User );":                         "UserRoles takes a list",
	}
	for src, want := range cases {
		t.Run(src, func(t *testing.T) {
			_, errs := Build(src)
			if len(errs) == 0 {
				t.Fatalf("no error, want one containing %q", want)
			}
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			if !strings.Contains(strings.Join(all, "\n"), want) {
				t.Errorf("errors %q do not contain %q", all, want)
			}
		})
	}
}

// The listener runs over statements the parser recovered from too, where a
// token the clause form requires is missing. The old describe of a demo user
// without roles (`… password '***';`) is one: it must be a syntax error, not a
// panic in the rewrite.
func TestHeaderClauseFormsRecoveredFromDoNotPanic(t *testing.T) {
	for _, src := range []string{
		"create or modify demo user 'u' password '***';",
		"create demo user 'u' password '***' entity (User);",
		"create demo user 'u' password (User);",
		"create constant M.C type String;",
		"create constant M.C type default 'x';",
		"create constant M.C type String default;",
	} {
		t.Run(src, func(t *testing.T) {
			if _, errs := Build(src); len(errs) == 0 {
				t.Errorf("no error for %q", src)
			}
		})
	}
}
