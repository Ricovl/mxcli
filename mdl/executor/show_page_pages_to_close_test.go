// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1617, the write half: `show page … with
// NumberOfPagesToClose = n` reaches the action. The `with` list read title,
// location and modal and ignored every other key, so the setting was dropped
// with `check` and `exec` both clean.
func TestBuildShowPage_NumberOfPagesToClose(t *testing.T) {
	fb := buildFlowFromMDL(t, "  show page MyFirstModule.Booking_Overview with NumberOfPagesToClose = 1;\n"+
		"  show page MyFirstModule.Booking_Overview with title = 'Bookings', NumberOfPagesToClose = $Count;\n"+
		"  show page MyFirstModule.Booking_Overview;")
	var got []string
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.ActionActivity); ok {
			if sp, ok := a.Action.(*microflows.ShowPageAction); ok {
				got = append(got, sp.NumberOfPagesToClose)
			}
		}
	}
	want := []string{"1", "$Count", ""}
	if len(got) != len(want) {
		t.Fatalf("built %d show page actions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("action %d: NumberOfPagesToClose = %q, want %q", i+1, got[i], want[i])
		}
	}
}

// What describe prints for the setting builds the setting it printed.
func TestShowPage_NumberOfPagesToCloseRoundTrips(t *testing.T) {
	e := newTestExecutor()
	stored := &microflows.ShowPageAction{PageName: "MyFirstModule.Booking_Overview", NumberOfPagesToClose: "1"}
	described := e.formatAction(stored, nil, nil)
	fb := buildFlowFromMDL(t, "  "+described)
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.ActionActivity); ok {
			if sp, ok := a.Action.(*microflows.ShowPageAction); ok {
				if sp.NumberOfPagesToClose != "1" {
					t.Errorf("%q built NumberOfPagesToClose %q, want \"1\"", described, sp.NumberOfPagesToClose)
				}
				return
			}
		}
	}
	t.Fatalf("%q built no show page action", described)
}
