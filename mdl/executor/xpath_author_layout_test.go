// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1542 (ako/mxcli#1440): a retrieve's XPath lost the layout
// its author gave it.
//
//	retrieve $Notes from M.Note where [IsOpen = true() and
//		Count > 0];
//
// was stored as `[IsOpen = true() and Count > 0]` — the line break and tab
// dropped — while every other expression slot keeps its line breaks. Studio Pro
// keeps the layout an author gives a constraint, so a describe/exec round trip
// changed the model. A constraint laid out over several lines is now stored as
// written; a flat one is laid out as before (upstream #979).

const authoredXPath = "[IsOpen = true() and\n\tCount > 0]"

func TestRetrieveXPath_KeepsTheAuthorsLineBreaks(t *testing.T) {
	cases := []struct{ name, where, want string }{
		{"line break and tab", "[IsOpen = true() and\n\tCount > 0]", authoredXPath},
		{"operator leading the next line", "[IsOpen = true()\n  and Count > 0]", "[IsOpen = true()\n  and Count > 0]"},
		{"operator case still folded", "[IsOpen = true() AND\n\tCount > 0]", authoredXPath},
		{"bare where", "IsOpen = true() and\n\tCount > 0", authoredXPath},
		// Controls: a flat constraint is stored as before.
		{"flat and short", "[IsOpen = true() and Count > 0]", "[IsOpen = true() and Count > 0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := retrieveConstraintOf(t, "mdl 1;\ncreate microflow M.F ()\nbegin\n  retrieve $Notes from M.Note where "+c.where+";\nend;\n")
			if got != c.want {
				t.Errorf("stored %q, want %q", got, c.want)
			}
		})
	}
}

// The flat control's long sibling: a constraint written on one line and too long
// to read there is still laid out by the writer (upstream #979).
func TestRetrieveXPath_FlatLongConstraintIsStillLaidOut(t *testing.T) {
	got := retrieveConstraintOf(t, "mdl 1;\ncreate microflow M.F ()\nbegin\n  retrieve $L from M.Req where [Archived = false and Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3];\nend;\n")
	if !strings.HasPrefix(got, "[\n  ") {
		t.Errorf("stored %q, want the #979 layout", got)
	}
}

// describe prints an authored layout as stored, and the description stores it
// again: the round trip the issue reports as changing the model.
func TestRetrieveXPath_AuthoredLayoutSurvivesDescribeAndExec(t *testing.T) {
	for _, stored := range []string{
		authoredXPath,
		"[IsOpen = true()\nand Count > 0]",
		// The writer's own layout of a long flat constraint: described flat, and
		// re-derived on write.
		"[\n  Archived = false\n  and Status = 'Open'\n  and Priority = 'High'\n  and Category = 'Electrical'\n  and Severity > 3\n]",
	} {
		e := newTestExecutor()
		described := e.formatAction(&microflows.RetrieveAction{
			OutputVariable: "Notes",
			Source: &microflows.DatabaseRetrieveSource{
				EntityQualifiedName: "M.Note",
				XPathConstraint:     stored,
			},
		}, nil, nil)
		got := retrieveConstraintOf(t, "mdl 1;\ncreate microflow M.F ()\nbegin\n  "+described+"\nend;\n")
		if got != stored {
			t.Errorf("stored %q\ndescribed as:\n%s\nre-executed it stores %q", stored, described, got)
		}
	}
}

// An access rule keeps its constraint's source text as the retrieve does, and
// shares the writer, so it keeps an authored layout too — and its describer
// must print it, or describe/exec of the rule would now change the model.
func TestAccessRuleXPath_AuthoredLayoutSurvivesDescribeAndExec(t *testing.T) {
	ctx := &ExecContext{LanguageVersion: langver.V1}
	for _, stored := range []string{
		authoredXPath,
		"[IsOpen = true()\nand Count > 0]",
		"[IsOpen = true()][Count > 0]",
		"[\n  Archived = false\n  and Status = 'Open'\n  and Priority = 'High'\n  and Category = 'Electrical'\n  and Severity > 3\n]",
	} {
		described := entityGrantMDL(ctx, "read *", "M.Note", []string{"M.R"}, stored)
		prog, errs := visitor.Build("mdl 1;\n" + described)
		if len(errs) > 0 {
			t.Fatalf("%q: description does not parse: %v\n%s", stored, errs, described)
		}
		written := prog.Statements[0].(*ast.GrantEntityAccessStmt).XPathConstraint
		if got := accessRuleXPathConstraint(nil, written, "M.Note"); got != stored {
			t.Errorf("stored %q\ndescribed as:\n%s\nre-executed it stores %q", stored, described, got)
		}
	}
}

// A Studio Pro constraint can end in a line break (WorkflowCommons'
// WorkflowUserTaskView rule does). Described as stored, the trailing newline
// made it fail IsBracketedXPath and fall back to the deprecated quoted form —
// which flattening used to hide by trimming. The layout inside the brackets
// is the author's; the whitespace outside them is nothing a bracketed
// constraint can say, and is dropped as before.
func TestAccessRuleXPath_TrailingLineBreakStaysBracketed(t *testing.T) {
	ctx := &ExecContext{LanguageVersion: langver.V1}
	stored := "[M.Note_Owner/M.Owner[Active = true()]]\n[IsOpen = true()\n\tor Count > 0]\n"
	described := entityGrantMDL(ctx, "read *", "M.Note", []string{"M.R"}, stored)
	if !strings.Contains(described, " where [") {
		t.Fatalf("described in the quoted form, want brackets:\n%s", described)
	}
	prog, errs := visitor.Build("mdl 1;\n" + described)
	if len(errs) > 0 {
		t.Fatalf("description does not parse: %v\n%s", errs, described)
	}
	written := prog.Statements[0].(*ast.GrantEntityAccessStmt).XPathConstraint
	if got, want := accessRuleXPathConstraint(nil, written, "M.Note"), strings.TrimSpace(stored); got != want {
		t.Errorf("re-executed it stores %q, want %q", got, want)
	}
}
