// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R6 (ako/mxcli#755): every old verb builds exactly the statement its
// canonical spelling builds, records its code, and the canonical spelling
// records nothing. The registry test covers one example per code; these cover
// every grammar alternative the alias appears in.
func TestR6OldVerbsAreAliases(t *testing.T) {
	cases := []struct {
		old, canonical, code string
	}{
		{"show page M.P;", "describe page M.P;", deprecation.ShowSingleThing},
		{"show project security;", "describe app security;", deprecation.ShowSingleThing},
		{"list project security;", "describe app security;", deprecation.ShowSingleThing},
		{"show security matrix;", "describe security matrix;", deprecation.ShowSingleThing},
		{"show security matrix in M;", "describe security matrix in M;", deprecation.ShowSingleThing},
		{"show structure;", "describe structure;", deprecation.ShowSingleThing},
		{"show structure depth 3 in M all;", "describe structure depth 3 in M all;", deprecation.ShowSingleThing},
		{"show context of M.F;", "describe context of M.F;", deprecation.ShowSingleThing},
		{"show context of M.F depth 4;", "describe context of M.F depth 4;", deprecation.ShowSingleThing},
		{"alter user role Clerk remove module roles (M.User, M.Admin);",
			"alter user role Clerk drop module roles (M.User, M.Admin);", deprecation.UserRoleRemove},
		{"alter settings language remove 'ar_SD';", "alter settings language drop 'ar_SD';", deprecation.SettingsRemove},
		{"alter settings workflows remove group 'Approvers';",
			"alter settings workflows drop group 'Approvers';", deprecation.SettingsRemove},
		{"alter entity M.E add column Note: String(200);", "alter entity M.E add attribute Note: String(200);",
			deprecation.ColumnForAttribute},
		{"alter entity M.E rename column Note to Remark;", "alter entity M.E rename attribute Note to Remark;",
			deprecation.ColumnForAttribute},
		{"alter entity M.E modify column Note: String(400);", "alter entity M.E modify attribute Note: String(400);",
			deprecation.ColumnForAttribute},
		{"alter entity M.E drop column if exists Note;", "alter entity M.E drop attribute if exists Note;",
			deprecation.ColumnForAttribute},
		{"create microflow M.F () begin rest call post 'https://x.org' body '{}' timeout 30 returns nothing; end;",
			"create microflow M.F () begin call rest service post 'https://x.org' body '{}' timeout 30 returns nothing; end;",
			deprecation.RestCall},
		{"describe widget combobox;", "describe widget type combobox;", deprecation.DescribeWidgetType},
		{"describe widget 'com.mendix.widget.web.combobox.Combobox';",
			"describe widget type 'com.mendix.widget.web.combobox.Combobox';", deprecation.DescribeWidgetType},
		{"define fragment Hdr ($p: datasource) as { dynamictext t (Content: 'x') };",
			"create fragment Hdr ($p: datasource) as { dynamictext t (Content: 'x') };", deprecation.DefineFragment},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{c.code}) {
				t.Errorf("old recorded %v, want [%s]", got, c.code)
			}
			canon := mustBuild(t, c.canonical)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical recorded %v, want none", got)
			}
			if len(old.Statements) != 1 || !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements:\n old:   %#v\n canon: %#v",
					old.Statements, canon.Statements)
			}
		})
	}
}

// `show entity|association|navigation|settings` print a summary where
// describe prints the definition as MDL: deprecated, but they keep their
// statement and carry no rewrite, only the reason.
func TestR6ShowSummariesAreReportedWithoutRewrite(t *testing.T) {
	for _, src := range []string{
		"show entity M.E;", "show association M.A;", "show navigation;",
		"show navigation menu Responsive;", "show settings;",
	} {
		t.Run(src, func(t *testing.T) {
			prog := mustBuild(t, src)
			if len(prog.Deprecations) != 1 || prog.Deprecations[0].Code != deprecation.ShowSingleThing {
				t.Fatalf("recorded %+v, want one %s", prog.Deprecations, deprecation.ShowSingleThing)
			}
			d := prog.Deprecations[0]
			if d.Fix != nil || d.NoFix == "" {
				t.Errorf("Fix = %+v, NoFix = %q; want no fix and a reason", d.Fix, d.NoFix)
			}
		})
	}
}

// The words that stay: a widget on a page (`describe styling … widget`,
// `describe fragment … widget`), `drop` as the canonical verb, and a plain
// `describe widget type`.
func TestR6CanonicalFormsRecordNothing(t *testing.T) {
	for _, src := range []string{
		"describe styling on page M.P widget btn1;",
		"describe fragment from page M.P widget btn1;",
		"describe widget type combobox;",
		"alter entity M.E drop default on attribute Note;",
		"create association M.A_B from M.A to M.B type Reference storage column;",
		"list navigation homes;",
	} {
		t.Run(src, func(t *testing.T) {
			if got := deprecationCodes(mustBuild(t, src)); len(got) != 0 {
				t.Errorf("recorded %v, want none", got)
			}
		})
	}
}
