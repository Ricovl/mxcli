// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): a workflow expression in a string is rewritten to the
// bare expression, keeping everything around it.
func TestUpgrade_WorkflowExpressionsBare(t *testing.T) {
	head := "create workflow M.WF\n  parameter $WorkflowContext: M.E\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			"decision, timers and due dates",
			head + "  due date 'addDays([%CurrentDateTime%], 2)'\nbegin\n" +
				"  user task Review 'Review'\n    due date 'addHours([%CurrentDateTime%], 4)'\n    outcomes 'Done' { }\n" +
				"    boundary event interrupting timer 'addHours([%CurrentDateTime%], 8)' { };\n" +
				"  decision '$WorkflowContext/Total > 1000' caption 'Large?' -- keep\n    outcomes true -> { } false -> { };\n" +
				"  wait for timer 'addHours([%CurrentDateTime%], 1)' caption 'Wait';\n" +
				"  event subprocess Esp on interrupting timer 'addDays([%CurrentDateTime%], 30)' as Start { };\nend workflow;",
			head + "  due date addDays([%CurrentDateTime%], 2)\nbegin\n" +
				"  user task Review 'Review'\n    due date addHours([%CurrentDateTime%], 4)\n    outcomes 'Done' { }\n" +
				"    boundary event interrupting timer addHours([%CurrentDateTime%], 8) { };\n" +
				"  decision $WorkflowContext/Total > 1000 caption 'Large?' -- keep\n    outcomes true -> { } false -> { };\n" +
				"  wait for timer addHours([%CurrentDateTime%], 1) caption 'Wait';\n" +
				"  event subprocess Esp on interrupting timer addDays([%CurrentDateTime%], 30) as Start { };\nend workflow;",
		},
		{
			// The first timer has no delay, and `non` must stay the start of
			// the second event rather than become the first one's delay.
			"a delayless timer before a non-interrupting one",
			head + "begin\n  user task Review 'Review'\n    outcomes 'Done' { }\n" +
				"    boundary event interrupting timer non interrupting timer 'addHours([%CurrentDateTime%], 1)';\nend workflow;",
			head + "begin\n  user task Review 'Review'\n    outcomes 'Done' { }\n" +
				"    boundary event interrupting timer non interrupting timer addHours([%CurrentDateTime%], 1);\nend workflow;",
		},
		{
			"a string with no space around it",
			// The old action form is rewritten as well (MDL-DEPR141, ako/mxcli#712).
			"alter workflow M.WF set activity Review due date'addDays([%CurrentDateTime%], 1)';",
			"alter workflow M.WF { set (DueDate: addDays([%CurrentDateTime%], 1)) on Review; };",
		},
		{
			"upper case, doubled quotes inside",
			"ALTER WORKFLOW M.WF SET DUE DATE 'if $x = ''a'' then [%CurrentDateTime%] else [%BeginOfCurrentDay%]';",
			"ALTER WORKFLOW M.WF { SET (DueDate: if $x = 'a' then [%CurrentDateTime%] else [%BeginOfCurrentDay%]); };",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mustUpgrade(t, tc.src, Options{})
			if res.Source != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", res.Source, tc.want)
			}
			if len(res.Unrewritten) != 0 {
				t.Errorf("Unrewritten = %+v", res.Unrewritten)
			}
		})
	}
}

// A string whose content is not a bare expression that reads back as itself
// is reported, not rewritten: unquoting it would change what is stored. So is
// a decision whose bare expression would be read as the decision's name.
func TestUpgrade_WorkflowExpressionNotBareIsReported(t *testing.T) {
	head := "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n"
	for _, src := range []string{
		"alter workflow M.WF { set (DueDate: '${PT1H}'); };",
		"alter workflow M.WF { set (DueDate: ' addDays([%CurrentDateTime%], 1)'); };",
		head + "  decision 'Total + 1 > 3'\n    outcomes true -> { } false -> { };\nend workflow;",
	} {
		res := mustUpgrade(t, src, Options{})
		if res.Source != src {
			t.Errorf("%q was rewritten to %q", src, res.Source)
		}
		if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.WorkflowStringExpression {
			t.Errorf("%q: Unrewritten = %+v, want one %s", src, res.Unrewritten, deprecation.WorkflowStringExpression)
		}
	}
}
