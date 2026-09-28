// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// r8Pair is one old spelling and its canonical form (R8, ako/mxcli#752). Both
// must parse, build the same statements, and only the old one may record code.
type r8Pair struct {
	name, old, canon, code string
}

func pageWith(action string) string {
	return "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { actionbutton b (Caption: 'Go', Action: " + action + ") };"
}

var r8Pairs = []r8Pair{
	{"save", pageWith("save_changes"), pageWith("save changes"), deprecation.PageActionWord},
	{"save close", pageWith("save_changes close_page"), pageWith("save changes close page"), deprecation.PageActionWord},
	{"save close upper", pageWith("SAVE_CHANGES CLOSE_PAGE"), pageWith("SAVE CHANGES CLOSE PAGE"), deprecation.PageActionWord},
	{"cancel close", pageWith("cancel_changes close_page"), pageWith("cancel changes close page"), deprecation.PageActionWord},
	{"close", pageWith("close_page"), pageWith("close page"), deprecation.PageActionWord},
	{"delete", pageWith("delete_object"), pageWith("delete"), deprecation.PageActionWord},
	{"delete close", pageWith("delete_object close_page"), pageWith("delete close page"), deprecation.PageActionWord},
	{"create then show", pageWith("create_object M.E then show_page M.Edit"), pageWith("create object M.E then show page M.Edit"), deprecation.PageActionWord},
	{"show page args", pageWith("show_page M.Edit(Item = $currentObject)"), pageWith("show page M.Edit(Item = $currentObject)"), deprecation.PageActionWord},
	{"microflow", pageWith("microflow M.ACT(X = 1)"), pageWith("call microflow M.ACT(X = 1)"), deprecation.PageActionWord},
	{"nanoflow", pageWith("nanoflow M.NF"), pageWith("call nanoflow M.NF"), deprecation.PageActionWord},
	{"open link", pageWith("open_link 'https://x.org'"), pageWith("open link 'https://x.org'"), deprecation.PageActionWord},
	{"open link attr", pageWith("open_link $currentObject/Url"), pageWith("open link $currentObject/Url"), deprecation.PageActionWord},
	{"sign out", pageWith("sign_out"), pageWith("sign out"), deprecation.PageActionWord},
	{"complete task", pageWith("complete_task 'Approve'"), pageWith("complete task 'Approve'"), deprecation.PageActionWord},
	{"menu sign out", "create navigation Responsive home page M.Home { menu item 'Out' ( OnClick: sign_out ) };",
		"create navigation Responsive home page M.Home { menu item 'Out' ( OnClick: sign out ) };", deprecation.PageActionWord},
	{"alter page set action", "alter page M.P { set (Action: show_page M.Q) on b };",
		"alter page M.P { set (Action: show page M.Q) on b };", deprecation.PageActionWord},

	{"not null error", "create entity M.E (Name: String(100) not null error 'Required');",
		"create entity M.E (Name: String(100) not null error message 'Required');", deprecation.ErrorMessageKeyword},
	{"unique error", "create entity M.E (Code: String(10) unique error 'Taken');",
		"create entity M.E (Code: String(10) unique error message 'Taken');", deprecation.ErrorMessageKeyword},
	{"validation feedback", "create validation rule for M.E.Email regex M.Pattern feedback 'Bad';",
		"create validation rule for M.E.Email regex M.Pattern error message 'Bad';", deprecation.ErrorMessageKeyword},
	{"association error_message", "create association M.A_B from M.A to M.B on delete restrict error_message 'In use';",
		"create association M.A_B from M.A to M.B on delete restrict error message 'In use';", deprecation.ErrorMessageKeyword},
	{"association errormessage", "create association M.A_B from M.A to M.B on delete restrict errormessage 'In use';",
		"create association M.A_B from M.A to M.B on delete restrict error message 'In use';", deprecation.ErrorMessageKeyword},

	{"delete_behavior cascade", "create association M.A_B from M.A to M.B delete_behavior cascade;",
		"create association M.A_B from M.A to M.B on delete cascade;", deprecation.DeleteBehaviorClause},
	{"delete_behavior delete_and_references", "create association M.A_B from M.A to M.B delete_behavior delete_and_references;",
		"create association M.A_B from M.A to M.B on delete cascade;", deprecation.DeleteBehaviorClause},
	{"deletebehavior deleteandreferences", "create association M.A_B from M.A to M.B DELETEBEHAVIOR DELETEANDREFERENCES;",
		"create association M.A_B from M.A to M.B ON DELETE CASCADE;", deprecation.DeleteBehaviorClause},
	{"delete_behavior delete but keep", "create association M.A_B from M.A to M.B delete_behavior delete but keep references;",
		"create association M.A_B from M.A to M.B on delete set null;", deprecation.DeleteBehaviorClause},
	{"delete_behavior if no refs", "create association M.A_B from M.A to M.B delete_behavior delete_if_no_references;",
		"create association M.A_B from M.A to M.B on delete restrict;", deprecation.DeleteBehaviorClause},
	{"alter set delete_behavior", "alter association M.A_B set delete_behavior prevent;",
		"alter association M.A_B set on delete restrict;", deprecation.DeleteBehaviorClause},

	{"reference_set", "create association M.A_B from M.A to M.B type reference_set;",
		"create association M.A_B from M.A to M.B type ReferenceSet;", deprecation.ReferenceSetUnderscore},

	{"returns none", "create microflow M.F () begin call rest service get 'https://x.org' returns none; end;",
		"create microflow M.F () begin call rest service get 'https://x.org' returns nothing; end;", deprecation.ReturnsNone},
}

func TestR8OldSpellingsAreAliases(t *testing.T) {
	for _, p := range r8Pairs {
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
			for _, c := range got {
				if c != p.code {
					t.Errorf("old %q recorded %v, want only %s", p.old, got, p.code)
				}
			}
			for _, d := range old.Deprecations {
				if d.Code == p.code && d.Fix == nil && d.Code != deprecation.ReturnsNone {
					t.Errorf("old %q: %s recorded without a rewrite (%s)", p.old, d.Code, d.NoFix)
				}
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old and canonical build different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The words are not a new meaning: every canonical page action builds the
// action its snake-case spelling always built.
func TestR8PageActionWordsBuildTheirAction(t *testing.T) {
	cases := map[string]ast.ActionV3{
		"save changes close page":   {Type: "save", ClosePage: true},
		"cancel changes":            {Type: "cancel"},
		"close page":                {Type: "close"},
		"delete close page":         {Type: "delete", ClosePage: true},
		"sign out":                  {Type: "signOut"},
		"complete task 'Approve'":   {Type: "completeTask", OutcomeValue: "Approve"},
		"open link 'https://x.org'": {Type: "openLink", LinkURL: "https://x.org"},
		"call microflow M.ACT":      {Type: "microflow", Target: "M.ACT"},
		"call nanoflow M.NF":        {Type: "nanoflow", Target: "M.NF"},
		"show page M.Edit":          {Type: "showPage", Target: "M.Edit"},
	}
	for src, want := range cases {
		prog := mustBuild(t, pageWith(src))
		stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
		got, ok := stmt.Widgets[0].Properties["Action"].(*ast.ActionV3)
		if !ok {
			t.Errorf("%s: Action is %T, want *ast.ActionV3", src, stmt.Widgets[0].Properties["Action"])
			continue
		}
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("%s: built %+v, want %+v", src, *got, want)
		}
	}
}
