// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// System members in a retrieve constraint, judged against an entity the
// PROJECT has (created by an earlier exec), on the Studio Pro-authored PedApp
// fixture. Measured on mxbuild 11.14.0 on a System.FileDocument
// specialization:
//
//	[System.owner = '[%CurrentUser%]']      clean
//	[System.changedBy = '[%CurrentUser%]']  clean
//	[owner = '[%CurrentUser%]']             CE0161
//	[System.Owner = '[%CurrentUser%]']      CE1613
//	[System.owner = …] on a plain entity    CE0161 (the control: it stores no owner)
//
// mxcli's check had the first two and the third exactly inverted: it listed
// bare owner/changedBy as implicit members, did not accept the qualified
// association, and read the system-member flags off the specialization rather
// than the root of its generalization chain.
func TestCheckSystemMemberXPath_OnAFileDocumentSpecialization(t *testing.T) {
	const setup = `mdl 1;
create module ModS;
create persistent entity ModS.Doc extends System.FileDocument (Title: String(100));
create persistent entity ModS.Plain (Name: String(100));`
	const flow = `mdl 1;
create microflow ModS.SUB_Find ()
begin
  retrieve $A from %s where %s;
end;`

	cases := []struct {
		entity, constraint string
		want               string // "" = must check clean
	}{
		{"ModS.Doc", "[System.owner = '[%CurrentUser%]']", ""},
		{"ModS.Doc", "[System.changedBy = '[%CurrentUser%]']", ""},
		{"ModS.Doc", "[System.owner/System.User/Name = 'x']", ""},
		{"ModS.Doc", "[createdDate < '[%CurrentDateTime%]']", ""},
		{"ModS.Doc", "[owner = '[%CurrentUser%]']", "spelled `System.owner`"},
		{"ModS.Doc", "[changedBy = '[%CurrentUser%]']", "spelled `System.changedBy`"},
		{"ModS.Doc", "[System.Owner = '[%CurrentUser%]']", "spelled `System.owner`"},
		// Control: an entity that stores no owner is still refused.
		{"ModS.Plain", "[System.owner = '[%CurrentUser%]']", "does not store owner"},
	}

	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range cases {
		src := strings.Replace(strings.Replace(flow, "%s", c.entity, 1), "%s", c.constraint, 1)
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		if c.want == "" {
			if got != "" {
				t.Errorf("%s %s: mxbuild builds this clean; check reported:\n%s", c.entity, c.constraint, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("%s %s: check must report %q, reported:\n%s", c.entity, c.constraint, c.want, got)
		}
	}
}

func TestSystemMemberSpellingHint(t *testing.T) {
	cases := map[string]string{
		"CreatedDate":      "`createdDate`",
		"owner":            "`System.owner`",
		"Owner":            "`System.owner`",
		"System.Owner":     "`System.owner`",
		"changedBy":        "`System.changedBy`",
		"createdDate":      "",
		"System.owner":     "",
		"SomethingElse":    "",
		"System.changedBy": "",
	}
	for in, want := range cases {
		got := systemMemberSpellingHint(in)
		if want == "" {
			if got != "" {
				t.Errorf("%q: no hint expected, got %q", in, got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("%q: hint %q does not name %s", in, got, want)
		}
	}
}
