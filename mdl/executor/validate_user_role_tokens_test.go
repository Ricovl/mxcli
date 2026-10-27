// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#1723: a `[%UserRole_X%]` token naming no user role passed check
// --references and exec, and mxbuild refused it as CE1613 — which also hid
// every CE0161 in the app.

func userRoleTokenCtx(t *testing.T) *ExecContext {
	t.Helper()
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{UserRoles: []*security.UserRole{{Name: "Administrator"}, {Name: "User"}}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func checkUserRoleTokens(t *testing.T, ctx *ExecContext, src string) ([]error, []string) {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateUserRoleTokens(ctx, prog, sc), sc.warnings
}

// userRoleSlots puts one role token in each XPath slot mxbuild resolves it in.
// The first four are measured (11.15.0): CE1613 at each with an unknown role.
func userRoleSlots(role string) map[string]string {
	tok := `'[%UserRole_` + role + `%]'`
	return map[string]string{
		"retrieve": `create microflow MyModule.R () returns Boolean
			begin
				retrieve $Users from System.User where [System.UserRoles = ` + tok + `];
				return true;
			end;`,
		"list view database source": `create page MyModule.Users (title: 'Users', layout: Atlas_Core.Atlas_Default) {
				listview lvUsers (datasource: database from System.User where [System.UserRoles = ` + tok + `]) {
					dynamictext txtName (Content: 'x')
				}
			};`,
		"access rule": `grant read * on entity MyModule.Doc to MyModule.Reader
			where [System.owner/System.User/System.UserRoles = ` + tok + `];`,
		"user task targeting": `create workflow MyModule.W
			  parameter $Context: MyModule.Doc
			begin
			  user task Review 'Review' page MyModule.TaskPage
			    targeting users xpath [System.UserRoles = ` + tok + `] outcomes 'Done' { };
			end workflow;`,
		"alter workflow targeting": `alter workflow MyModule.W { set (Targeting: xpath [System.UserRoles = ` + tok + `]) on Review; };`,
		"offline sync": `create or modify navigation "PhoneOffline"
			  sync ( sync MyModule.Doc where [System.owner/System.User/System.UserRoles = ` + tok + `]; );`,
	}
}

func TestUserRoleTokens_ReportsAnUnknownRoleInEverySlot(t *testing.T) {
	for slot, src := range userRoleSlots("Manager") {
		t.Run(slot, func(t *testing.T) {
			errs, _ := checkUserRoleTokens(t, userRoleTokenCtx(t), src)
			if len(errs) != 1 {
				t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
			}
			for _, w := range []string{"[%UserRole_Manager%]", "CE1613", "Administrator, User"} {
				if !strings.Contains(errs[0].Error(), w) {
					t.Errorf("error should mention %s: %v", w, errs[0])
				}
			}
		})
	}
}

// CONTROL: the same slots with a role the project has.
func TestUserRoleTokens_SilentForAKnownRole(t *testing.T) {
	for slot, src := range userRoleSlots("User") {
		t.Run(slot, func(t *testing.T) {
			if errs, warns := checkUserRoleTokens(t, userRoleTokenCtx(t), src); len(errs)+len(warns) != 0 {
				t.Errorf("reported a role the project has: %v %v", errs, warns)
			}
		})
	}
}

// Measured: 'UserRole_user' is CE1613 where 'UserRole_User' is clean.
func TestUserRoleTokens_TheNameIsCaseSensitive(t *testing.T) {
	errs, _ := checkUserRoleTokens(t, userRoleTokenCtx(t), userRoleSlots("user")["retrieve"])
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "case-sensitive: [%UserRole_User%]") {
		t.Errorf("want one error naming the right spelling, got %v", errs)
	}
}

func TestUserRoleTokens_ARoleTheScriptCreatesIsKnown(t *testing.T) {
	src := `create user role Manager (ModuleRoles: (MyModule.Reader));
` + userRoleSlots("Manager")["retrieve"]
	if errs, _ := checkUserRoleTokens(t, userRoleTokenCtx(t), src); len(errs) != 0 {
		t.Errorf("reported a role the script creates: %v", errs)
	}
}

// What cannot be established is silence: no project, an unreadable security
// document, and tokens that are not user roles.
func TestUserRoleTokens_SilentWhenItCannotJudge(t *testing.T) {
	src := userRoleSlots("Manager")["retrieve"]

	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return false }}))
	if errs, _ := checkUserRoleTokens(t, ctx, src); len(errs) != 0 {
		t.Errorf("no project: %v", errs)
	}

	ctx, _ = newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return nil, nil },
	}))
	if errs, _ := checkUserRoleTokens(t, ctx, src); len(errs) != 0 {
		t.Errorf("no security document: %v", errs)
	}

	other := `create microflow MyModule.R () returns Boolean
		begin
			retrieve $Docs from MyModule.Doc where [System.owner = '[%CurrentUser%]'];
			return true;
		end;`
	if errs, _ := checkUserRoleTokens(t, userRoleTokenCtx(t), other); len(errs) != 0 {
		t.Errorf("a token that is not a user role: %v", errs)
	}
}

// Mendix does not validate an excluded document: a warning, not an error.
func TestUserRoleTokens_ExcludedFlowWarns(t *testing.T) {
	errs, warns := checkUserRoleTokens(t, userRoleTokenCtx(t), "@excluded\n"+userRoleSlots("Manager")["retrieve"])
	if len(errs) != 0 || len(warns) != 1 {
		t.Errorf("excluded: got errors %v, warnings %v; want one warning", errs, warns)
	}
}

// The check is wired into the --references pass, which exec runs too.
func TestUserRoleTokens_RunsInReferencePass(t *testing.T) {
	errs, _ := validateProgramWithWarnings(userRoleTokenCtx(t), parseMDL(t, userRoleSlots("Manager")["access rule"]))
	for _, e := range errs {
		if strings.Contains(e.Error(), "[%UserRole_Manager%]") {
			return
		}
	}
	t.Fatalf("reference pass did not report [%%UserRole_Manager%%]: %v", errs)
}
