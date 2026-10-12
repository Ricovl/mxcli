// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1252: the reader dropped ErrorHandlingType off a Rollback and
// a Show home page action. DESCRIBE gates an activity's whole error branch on
// that value, so a Studio Pro handler on either came back as nothing at all.
func TestActionFromGen_1252_ReadsErrorHandlingType(t *testing.T) {
	rb := decodeAction(t, bson.D{
		{Key: "$ID", Value: "rb-1"},
		{Key: "$Type", Value: "Microflows$RollbackAction"},
		{Key: "ErrorHandlingType", Value: "Custom"},
		{Key: "RefreshInClient", Value: false},
		{Key: "RollbackVariableName", Value: "Car"},
	})
	if got := microflows.ActionErrorHandlingType(rb); got != microflows.ErrorHandlingTypeCustom {
		t.Errorf("rollback: ErrorHandlingType = %q, want Custom", got)
	}

	hp := decodeAction(t, bson.D{
		{Key: "$ID", Value: "hp-1"},
		{Key: "$Type", Value: "Microflows$ShowHomePageAction"},
		{Key: "ErrorHandlingType", Value: "CustomWithoutRollBack"},
	})
	if got := microflows.ActionErrorHandlingType(hp); got != microflows.ErrorHandlingTypeCustomWithoutRollback {
		t.Errorf("show home page: ErrorHandlingType = %q, want CustomWithoutRollBack", got)
	}
}

// The write side: an authored clause reaches the BSON, and no clause leaves the
// rollback without the key, as it has always been written.
func TestMicroflowRoundTrip_1252_RollbackAndHomePageErrorHandling(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action microflows.MicroflowAction
		want   microflows.ErrorHandlingType
	}{
		{"rollback custom", &microflows.RollbackObjectAction{RollbackVariable: "Car",
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom}, microflows.ErrorHandlingTypeCustom},
		{"rollback no clause", &microflows.RollbackObjectAction{RollbackVariable: "Car"}, ""},
		{"home page continue", &microflows.ShowHomePageAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeContinue}, microflows.ErrorHandlingTypeContinue},
		// Show home page has always been written with a literal Rollback.
		{"home page no clause", &microflows.ShowHomePageAction{}, microflows.ErrorHandlingTypeRollback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			switch a := tc.action.(type) {
			case *microflows.RollbackObjectAction:
				a.ID = model.ID("a-1")
			case *microflows.ShowHomePageAction:
				a.ID = model.ID("a-1")
			}
			activity := &microflows.ActionActivity{Action: tc.action}
			activity.ID = model.ID("act-1")
			mf := &microflows.Microflow{
				Name:             "ACT_T",
				ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{activity}},
			}
			mf.ID = model.ID("mf-1")

			got := roundTripMicroflow(t, mf)
			for _, obj := range got.ObjectCollection.Objects {
				if aa, ok := obj.(*microflows.ActionActivity); ok {
					if eh := microflows.ActionErrorHandlingType(aa.Action); eh != tc.want {
						t.Errorf("ErrorHandlingType = %q, want %q", eh, tc.want)
					}
					return
				}
			}
			t.Fatal("no action activity after the round trip")
		})
	}
}
