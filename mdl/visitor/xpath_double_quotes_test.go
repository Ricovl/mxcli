// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1243: a double-quoted name inside a GRANT … WHERE XPath was
// stored verbatim. In Mendix XPath `"Status"` is then a string literal, so
// `"Status" = 'Accepted'` is always false and the access rule silently grants
// no rows — check, exec and mx check all passed. A retrieve's XPath already
// strips identifier quotes (2026-07-08 finding); the grant now does the same,
// in both spellings and nested predicates included.
func TestGrantWhereStripsIdentifierQuotes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`mdl 1; grant read * on entity Rp.Item to Rp.R2 where ["Status" = 'Accepted'];`,
			`[Status = 'Accepted']`},
		{`mdl 1; grant read * on entity Rp.Item to Rp.R4 where ["Status" = 'Accepted' and Rp.Item_Parent/Rp.Parent["Name" = 'x']];`,
			`[Status = 'Accepted' and Rp.Item_Parent/Rp.Parent[Name = 'x']]`},
		{`GRANT Rp.R3 ON Rp.Item (READ *) WHERE '[Kind = ''a'' and "Status" = ''Accepted'']';`,
			`[Kind = 'a' and Status = 'Accepted']`},
		// Control: a double quote inside a string literal is content.
		{`mdl 1; grant read * on entity Rp.Item to Rp.R1 where [Kind = 'say "hi"'];`,
			`[Kind = 'say "hi"']`},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Errorf("%s: %v", tc.src, errs)
			continue
		}
		g, ok := prog.Statements[len(prog.Statements)-1].(*ast.GrantEntityAccessStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[len(prog.Statements)-1])
		}
		if g.XPathConstraint != tc.want {
			t.Errorf("%s:\n got  %s\n want %s", tc.src, g.XPathConstraint, tc.want)
		}
	}
}

// ako/mxcli#566 (ask 2): a double-quoted VALUE — `Name = "ServiceManager"` — had
// its quotes stripped like a name and was stored as the bare token
// `Name = ServiceManager`, a member path rather than the string the author
// meant (Mendix rejects `"…"` as a string: CE0161). The right-hand side of a
// comparison in Mendix XPath is never a member name, so a quoted token there is
// refused, in every XPath position and both spellings.
func TestXPathDoubleQuotedValueIsRefused(t *testing.T) {
	flow := func(where string) string {
		return "create microflow M.F ()\nbegin\n  retrieve $L from System.User where " + where + ";\nend;\n"
	}
	for _, src := range []string{
		flow(`[System.UserRoles/System.UserRole/Name = "ServiceManager"]`),
		"mdl 1;\n" + flow(`[System.UserRoles/System.UserRole/Name = "ServiceManager"]`),
		flow(`'[System.UserRoles/System.UserRole/Name = "ServiceManager"]'`),
		`mdl 1; grant read * on entity Rp.Item to Rp.R2 where [Kind != "ServiceManager"];`,
		`GRANT Rp.R3 ON Rp.Item (READ *) WHERE '[Kind = "ServiceManager"]';`,
	} {
		_, errs := Build(src)
		var hit bool
		for _, e := range errs {
			if strings.Contains(e.Error(), `"ServiceManager"`) && strings.Contains(e.Error(), `'ServiceManager'`) && !strings.Contains(e.Error(), "'''") {
				hit = true
			}
		}
		if !hit {
			t.Errorf("not refused with a pointer to the single-quoted form:\n%s\nerrors: %v", src, errs)
		}
	}

	// Controls: a quoted NAME on the left, and a double quote inside a
	// string literal, are not values.
	for _, src := range []string{
		flow(`["Name" = 'ServiceManager']`),
		flow(`[Name = 'say "hi"']`),
		flow(`[contains("Name", 'x')]`),
	} {
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("refused a valid XPath:\n%s\n%v", src, errs)
		}
	}
}

// The workflow targeting XPath is the same sink, in both spellings.
func TestWorkflowTargetingXPathQuotes(t *testing.T) {
	wf := func(targeting string) string {
		return "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" +
			"  user task Review 'Review'\n    page M.ReviewPage\n    " + targeting + ";\nend workflow;"
	}
	for _, src := range []string{
		wf(`targeting users xpath [Name = "Admin"]`),
		wf(`targeting users xpath '[Name = "Admin"]'`),
	} {
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("double-quoted value not refused:\n%s", src)
		}
	}
	for _, targeting := range []string{
		`targeting users xpath ["Name" = 'Admin']`,
		`targeting users xpath '["Name" = ''Admin'']'`,
	} {
		prog, errs := Build(wf(targeting))
		if len(errs) > 0 {
			t.Fatalf("%s: %v", targeting, errs)
		}
		var got string
		for _, s := range prog.Statements {
			if w, ok := s.(*ast.CreateWorkflowStmt); ok {
				for _, a := range w.Activities {
					if u, ok := a.(*ast.WorkflowUserTaskNode); ok && u.Targeting.XPath != "" {
						got = u.Targeting.XPath
					}
				}
			}
		}
		if got != `[Name = 'Admin']` {
			t.Errorf("%s: stored %q, want [Name = 'Admin']", targeting, got)
		}
	}
}
