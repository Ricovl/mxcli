// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#877: check warns on a doc comment the statement after it ignores,
// naming the statement that would have stored it. Control: the same comment
// above the create is not reported.
func TestValidateProgram_DetachedDocComment(t *testing.T) {
	const lost = "/** Fetches the orders. */\ndrop microflow if exists Rest.GetOrders;\n" +
		"create microflow Rest.GetOrders ($Id: Integer)\nbegin\nend;\n"
	const kept = "drop microflow if exists Rest.GetOrders;\n/** Fetches the orders. */\n" +
		"create microflow Rest.GetOrders ($Id: Integer)\nbegin\nend;\n"
	count := func(src string) []string {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var msgs []string
		for _, v := range ValidateProgram(prog, "") {
			if v.RuleID == detachedDocCommentRule {
				msgs = append(msgs, v.Message+" | "+v.Suggestion)
			}
		}
		return msgs
	}
	got := count(lost)
	if len(got) != 1 {
		t.Fatalf("lost comment: got %d MDL089 warnings, want 1: %v", len(got), got)
	}
	for _, want := range []string{"line 1", "drop microflow if exists Rest.GetOrders", "create microflow Rest.GetOrders` (line 3)"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("want %q in %q", want, got[0])
		}
	}
	if got := count(kept); len(got) != 0 {
		t.Errorf("a comment on its create was reported: %v", got)
	}
}
