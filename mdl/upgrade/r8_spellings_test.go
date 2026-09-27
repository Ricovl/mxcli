// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"
)

// R8 (ako/mxcli#752): every old spelling upgrades to exactly its canonical
// form, keeping the letter case it was written in. The visitor test
// (mdl/visitor/r8_spellings_test.go) proves each pair builds the same
// statements; this one proves the rewrite produces that pair.
func TestUpgrade_R8Spellings(t *testing.T) {
	page := func(action string) string {
		return "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
			"  actionbutton b (Caption: 'Go', Action: " + action + ")\n};\n"
	}
	cases := []struct{ old, want string }{
		{page("save_changes close_page"), page("save changes close page")},
		{page("SAVE_CHANGES CLOSE_PAGE"), page("SAVE CHANGES CLOSE PAGE")},
		{page("cancel_changes"), page("cancel changes")},
		{page("delete_object close_page"), page("delete close page")},
		{page("DELETE_OBJECT"), page("DELETE")},
		{page("create_object M.E then show_page M.Edit(Item: $currentObject)"), page("create object M.E then show page M.Edit(Item = $currentObject)")},
		{page("microflow M.ACT(X: 1)"), page("call microflow M.ACT(X = 1)")},
		{page("NANOFLOW M.NF"), page("CALL NANOFLOW M.NF")},
		{page("open_link $currentObject/Url"), page("open link $currentObject/Url")},
		{page("sign_out"), page("sign out")},
		{page("complete_task 'Approve'"), page("complete task 'Approve'")},
		{"create navigation Responsive home page M.Home menu (menu item 'Out' sign_out;);\n",
			"create navigation Responsive home page M.Home menu (menu item 'Out' sign out;);\n"},
		{"create entity M.E (\n  Name: String(100) not null error 'Required',\n  Code: String(9) UNIQUE ERROR 'Taken'\n);\n",
			"create entity M.E (\n  Name: String(100) not null error message 'Required',\n  Code: String(9) UNIQUE ERROR MESSAGE 'Taken'\n);\n"},
		{"create validation rule for M.E.Email regex M.Pattern\n  feedback 'Bad';\n",
			"create validation rule for M.E.Email regex M.Pattern\n  error message 'Bad';\n"},
		{"create association M.A_B from M.A to M.B type reference_set on delete restrict error_message 'In use';\n",
			"create association M.A_B from M.A to M.B type ReferenceSet on delete restrict error message 'In use';\n"},
		{"create association M.A_B from M.A to M.B DELETE_BEHAVIOR DELETE_AND_REFERENCES;\n",
			"create association M.A_B from M.A to M.B ON DELETE CASCADE;\n"},
		{"alter association M.A_B set delete_behavior delete but keep references error_message 'x';\n",
			"alter association M.A_B set on delete set null error message 'x';\n"},
		{"create microflow M.F () begin rest call get 'https://x.org' returns none; end;\n",
			"create microflow M.F () begin rest call get 'https://x.org' returns nothing; end;\n"},
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
