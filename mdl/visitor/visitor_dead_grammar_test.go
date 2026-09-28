// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// Plan item 2.3 of PROPOSAL_mdl_beta_syntax_freeze.md (ako/mxcli#756): grammar
// that parsed and could never succeed is removed, and what used to be a late
// refusal is a parse error that says why and what to write instead.
//
//   - `grant|revoke execute on workflow` parsed and was refused by exec: a
//     Mendix workflow has no allowed roles of its own.
//   - `case … else` parsed, `check` refused it (MDL008) and exec wrote an
//     enumeration split mxbuild rejects (CE0079, CE0773): an enumeration split
//     has no default flow.
//
// Each has a control that differs only in the removed part and still parses.
func TestDeadGrammarIsAParseErrorWithAHint(t *testing.T) {
	for _, tc := range []struct {
		name, removed, control, hint string
	}{
		{
			name:    "grant execute on workflow",
			removed: "grant execute on workflow M.Approve to M.Manager;",
			control: "grant execute on microflow M.Approve to M.Manager;",
			hint:    "workflow has no allowed roles",
		},
		{
			name:    "revoke execute on workflow",
			removed: "revoke execute on workflow M.Approve from M.Manager;",
			control: "revoke execute on microflow M.Approve from M.Manager;",
			hint:    "workflow has no allowed roles",
		},
		{
			name: "case else",
			removed: "create microflow M.F ($S: Enumeration(M.Status)) begin\n" +
				"  case $S\n    when Open then log info 'o';\n    else log info 'x';\n  end case;\nend;",
			control: "create microflow M.F ($S: Enumeration(M.Status)) begin\n" +
				"  case $S\n    when Open then log info 'o';\n    when (empty) then log info 'x';\n  end case;\nend;",
			hint: "no default branch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, errs := Build(tc.control); len(errs) > 0 {
				t.Fatalf("control does not parse: %v", errs[0])
			}
			_, errs := Build(tc.removed)
			if len(errs) == 0 {
				t.Fatalf("%q still parses", tc.removed)
			}
			all := ""
			for _, e := range errs {
				all += e.Error() + "\n"
			}
			if !strings.Contains(all, tc.hint) {
				t.Errorf("the parse error does not explain the removal (want %q):\n%s", tc.hint, all)
			}
		})
	}
}
