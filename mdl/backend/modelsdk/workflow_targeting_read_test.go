// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue ako/mxcli#1722. `targeting groups xpath [...]` was written as
// Workflows$XPathGroupTargeting and read back as no targeting: the reader had a
// case for every other targeting kind but that one. DESCRIBE printed the task
// without the clause, so a describe -> create or modify round trip removed it.
// Every kind the writer emits must read back as itself.
func TestWorkflowRead_UserTaskTargetingRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source workflows.UserSource
	}{
		{"users microflow", &workflows.MicroflowBasedUserSource{Microflow: "M.ACT_Users"}},
		{"users xpath", &workflows.XPathBasedUserSource{XPath: "[Active = true()]"}},
		{"groups microflow", &workflows.MicroflowGroupSource{Microflow: "M.ACT_Groups"}},
		{"groups xpath", &workflows.XPathGroupSource{XPath: "[Name = 'Reviewers']"}},
		{"none", &workflows.NoUserSource{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, multi := range []bool{false, true} {
				ut := &workflows.UserTask{Page: "M.TaskPage", IsMulti: multi, UserSource: tc.source}
				ut.Name, ut.Caption = "Review", "Review"
				ut.Outcomes = []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}}

				rt, ok := roundTripWorkflowActivity(t, ut).(*workflows.UserTask)
				if !ok {
					t.Fatalf("multi=%v: round-tripped to a different type", multi)
				}
				if !reflect.DeepEqual(rt.UserSource, tc.source) {
					t.Errorf("multi=%v: targeting read back as %#v, want %#v", multi, rt.UserSource, tc.source)
				}
			}
		})
	}
}
