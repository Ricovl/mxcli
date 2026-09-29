// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"strings"
	"testing"
)

// Studio Pro-authored text templates that end in or contain a line break
// (#746): a lone template (PedApp's validation feedback, TestApp's show
// message) and templates with parameters (TestApp's log and show message).
var templateLineBreakCases = []struct {
	fx       fixture
	target   string
	flow     string // the unit's name, for flowUnit
	template string // the template as describe under mdl 1 must print it
}{
	{pedApp, valFeedback, "VAL_Feedback",
		"message 'Subject length cannot be longer than 200 characters\n';"},
	{testApp, "microflow WorkflowCommons.ACT_UserTask_AssignToUsers", "ACT_UserTask_AssignToUsers",
		"show message 'This action is only available for multi-user tasks.\n' type Error blocking;"},
	{testApp, "microflow WorkflowCommons.SUB_WorkflowAuditTrailRecord_CleanUp", "SUB_WorkflowAuditTrailRecord_CleanUp",
		"'Deleted {1} expired audit trail records for {2} workflow instances.\n' with ({1} = "},
	{testApp, "microflow WorkflowCommons.ACT_TaskAssignmentHelper_Reassign", "ACT_TaskAssignmentHelper_Reassign",
		"show message 'Successfully re-assigned {1} task(s). \n' type Information with ({1} = "},
}

// Under mdl 1 describe writes a template's line break into the literal, and
// executing that description under the header writes nothing: the literal
// reads back as the stored template text, not as a `{1}` parameter and not as
// a backslash and an n.
func TestTemplateLineBreakRoundTripsUnderMdl1(t *testing.T) {
	harnesses := map[string]*harness{}
	defer func() {
		for _, h := range harnesses {
			h.close()
		}
	}()
	for _, c := range templateLineBreakCases {
		t.Run(c.target, func(t *testing.T) {
			h := harnesses[c.fx.name]
			if h == nil {
				h = newFixtureHarness(t, c.fx)
				harnesses[c.fx.name] = h
			}
			h.restore()
			first := h.describeUnder("mdl 1;", c.target)
			if !strings.Contains(first, c.template) {
				t.Fatalf("describe under mdl 1 does not print %q:\n%s", c.template, first)
			}

			before := h.flowUnit(t, c.flow)
			if err := h.exec("mdl 1;\n" + first); err != nil {
				t.Fatalf("exec the mdl 1 description: %v\n%s", err, first)
			}
			if !bytes.Equal(h.flowUnit(t, c.flow), before) {
				t.Errorf("the mdl 1 description of an unchanged flow wrote it:\n%s", h.out.String())
			}
			if again := h.describeUnder("mdl 1;", c.target); again != first {
				t.Errorf("describe -> exec -> describe changed the flow:\n%s", lineDiff(first, again))
			}
		})
	}
}

// Control: the mdl 0 description, whose `\n` is a backslash and an n under
// mdl 1, does write the flow when executed under the header, so the byte
// comparison above can fail.
func TestTemplateLineBreakRoundTripsUnderMdl1_Control(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	plain := h.mustDescribe(t, valFeedback)
	before := h.flowUnit(t, "VAL_Feedback")
	if err := h.exec("mdl 1;\n" + plain); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(h.flowUnit(t, "VAL_Feedback"), before) {
		t.Fatal("the mdl 0 description executed under mdl 1 wrote nothing — the comparison cannot fail")
	}
}
