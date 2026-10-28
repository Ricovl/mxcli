// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1531: describe printed `call workflow M.WF_Child;` for a call
// stored with an association-path mapping. Re-executing it let the executor
// bind the default `$WorkflowContext` — the parent's context object — so the
// describe -> exec loop silently replaced the mapping (CE0117 on 11.15.0).
func TestCallWorkflowMappingSurvivesDescribeExec(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const mapping = "(WorkflowContext = $WorkflowContext/CWF1531.Parent_Current/CWF1531.Item)"
	if err := h.exec(`create module CWF1531;
create persistent entity CWF1531.Item (Name: string(100));
create persistent entity CWF1531.Parent (Name: string(100));
create association CWF1531.Parent_Current from CWF1531.Parent to CWF1531.Item;
create workflow CWF1531.WF_Child
  parameter $WorkflowContext: CWF1531.Item
begin
end workflow;
create workflow CWF1531.WF_Parent
  parameter $WorkflowContext: CWF1531.Parent
begin
  call workflow CWF1531.WF_Child` + mapping + `;
end workflow;`); err != nil {
		t.Fatalf("setup: %v", err)
	}

	first := h.mustDescribeMdl0(t, "workflow CWF1531.WF_Parent")
	if !strings.Contains(first, "call workflow CWF1531.WF_Child"+mapping) {
		t.Fatalf("describe dropped the parameter mapping:\n%s", first)
	}

	if err := h.exec(first); err != nil {
		t.Fatalf("re-exec describe output: %v", err)
	}
	if out := h.out.String(); !strings.Contains(out, "Unchanged workflow") {
		t.Errorf("re-executing the describe output wrote the workflow:\n%s", out)
	}
	if second := h.mustDescribeMdl0(t, "workflow CWF1531.WF_Parent"); second != first {
		t.Errorf("describe changed after describe -> exec:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
