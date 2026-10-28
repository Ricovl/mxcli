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

// mendixlabs/mxcli#1510: a doc comment above a create that stores none was
// told to move "directly above" the next create — documenting a different
// element with text written for this one. A drop or grant stands between a
// comment and its create; a create IS the element the comment describes, so
// the next statement is not where it belongs. Control: a drop still gets the
// move suggestion.
func TestValidateProgram_DetachedDocCommentOnCreate_Issue1510(t *testing.T) {
	const src = "/** Sales staff. */\ncreate module role Sales.User;\n" +
		"/** An order. */\ncreate persistent entity Sales.Order (Name: String(20));\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var got []string
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == detachedDocCommentRule {
			got = append(got, v.Message+" | "+v.Suggestion)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d MDL089 warnings, want 1: %v", len(got), got)
	}
	for _, bad := range []string{"Move the doc comment directly above", "next statement that can take it",
		"create persistent entity Sales.Order"} {
		if strings.Contains(got[0], bad) {
			t.Errorf("the warning sends the comment to a different element (%q): %s", bad, got[0])
		}
	}
	for _, want := range []string{"create module role Sales.User", "`--` comment"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("want %q in %q", want, got[0])
		}
	}
}
