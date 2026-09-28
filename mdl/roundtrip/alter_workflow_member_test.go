// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"
)

// ako/mxcli#791, on a Studio Pro-authored workflow: TestApp's
// workflow.Workflow1 has a user task, userTask1, with the outcomes 'Good',
// 'Bad' and 'Ugly'. `drop userTask1 path 1` removed 'Good' and printed
// "Altered workflow"; the old `drop path '' on userTask1` removed 'Ugly'. A
// user task has no paths, so both are refused with nothing written.

const testAppWorkflow = "workflow workflow.Workflow1"

func TestTestAppAlterWorkflow_DropPathOnUserTaskIsRefused(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	before := h.mustDescribe(t, testAppWorkflow)
	for _, want := range []string{"'Good' { }", "'Bad' { }", "'Ugly' { }"} {
		if !strings.Contains(before, want) {
			t.Fatalf("fixture changed: describe has no %s:\n%s", want, before)
		}
	}

	for _, tc := range []struct{ script, want string }{
		{"alter workflow workflow.Workflow1 { drop userTask1 path 1 };", "not a parallel split"},
		{"alter workflow workflow.Workflow1 { drop userTask2 path 1 };", "not a parallel split"},
		{"alter workflow workflow.Workflow1 drop path 'Path 1' on userTask1;", "not a parallel split"},
		{"alter workflow workflow.Workflow1 drop path '' on userTask1;", "not a parallel split"},
		{"alter workflow workflow.Workflow1 { drop aiAgentTask1 path 1 };", "not a parallel split"},
		{"alter workflow workflow.Workflow1 { insert into userTask1 { path { } } };", "cannot hold"},
		// Sibling found in review: an empty value matched aiAgentTask1's void
		// (default) outcome, which stores no value, and dropped it.
		{"alter workflow workflow.Workflow1 { drop aiAgentTask1 outcome '' };", "not found"},
		{"alter workflow workflow.Workflow1 drop condition '' on aiAgentTask1;", "not found"},
	} {
		err := h.exec(tc.script)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s\n  want a refusal containing %q, got %v", tc.script, tc.want, err)
		}
		if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
			t.Fatalf("%s\n  the refused statement wrote: %s", tc.script, strings.Join(changed, "; "))
		}
	}
	if after := h.mustDescribe(t, testAppWorkflow); after != before {
		t.Errorf("describe changed:\n--- before\n%s\n--- after\n%s", before, after)
	}

	// Control: the same pipeline does write when the op addresses what it
	// can hold — an outcome dropped by its value.
	if err := h.exec("alter workflow workflow.Workflow1 { drop userTask1 outcome 'Good' };"); err != nil {
		t.Fatalf("control: %v", err)
	}
	after := h.mustDescribe(t, testAppWorkflow)
	if strings.Contains(after, "'Good' { }") || !strings.Contains(after, "'Bad' { }") || !strings.Contains(after, "'Ugly' { }") {
		t.Errorf("control: want only 'Good' dropped:\n%s", after)
	}
}
