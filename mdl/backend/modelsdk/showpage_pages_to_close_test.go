// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1617: a microflow Show page action's "Number of pages to
// close" (Microflows$ShowFormAction.NumberOfPagesToClose, a microflow
// expression; "" closes nothing) was never read, and the writer stored "" in
// every action it built. DESCRIBE therefore printed an action that closes one
// page exactly like one that closes none, and no statement could write one.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// studioProShowFormAction is a Show page action as Studio Pro stores it with
// "Number of pages to close" set (bson dump of the #1617 repro).
func studioProShowFormAction(pagesToClose string) bson.D {
	return bson.D{
		{Key: "$ID", Value: "a-1"},
		{Key: "$Type", Value: "Microflows$ShowFormAction"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "FormSettings", Value: bson.D{
			{Key: "$ID", Value: "fs-1"},
			{Key: "$Type", Value: "Forms$FormSettings"},
			{Key: "Form", Value: "MyFirstModule.Booking_Overview"},
			{Key: "ParameterMappings", Value: bson.A{int32(2)}},
			{Key: "TitleOverride", Value: nil},
		}},
		{Key: "NumberOfPagesToClose", Value: pagesToClose},
	}
}

func TestShowPage_NumberOfPagesToCloseIsRead(t *testing.T) {
	got, ok := decodeAction(t, studioProShowFormAction("1")).(*microflows.ShowPageAction)
	if !ok {
		t.Fatalf("read back as %T", got)
	}
	if got.NumberOfPagesToClose != "1" {
		t.Errorf("NumberOfPagesToClose = %q, want \"1\": an action that closes a page reads back as one "+
			"that closes none (mendixlabs/mxcli#1617)", got.NumberOfPagesToClose)
	}
	// CONTROL: the stored default reads back as no pages to close, so the
	// assertion above is about the value and not a constant the reader returns.
	none, _ := decodeAction(t, studioProShowFormAction("")).(*microflows.ShowPageAction)
	if none == nil || none.NumberOfPagesToClose != "" {
		t.Errorf("the default \"\" read back as %+v", none)
	}
}

func TestShowPage_NumberOfPagesToCloseIsWritten(t *testing.T) {
	d := encodeDoc(t, microflowActionToGen(&microflows.ShowPageAction{
		PageName: "MyFirstModule.Booking_Overview", NumberOfPagesToClose: "1",
	}))
	if v, _ := lookupKey(d, "NumberOfPagesToClose"); v != "1" {
		t.Errorf("NumberOfPagesToClose = %#v, want \"1\": the writer stored \"\" whatever the statement "+
			"said (mendixlabs/mxcli#1617)", v)
	}
	// CONTROL: an action that closes nothing keeps the key, as Studio Pro
	// stores it, with the empty expression.
	d = encodeDoc(t, microflowActionToGen(&microflows.ShowPageAction{PageName: "MyFirstModule.Booking_Overview"}))
	if v, ok := lookupKey(d, "NumberOfPagesToClose"); !ok || v != "" {
		t.Errorf("no pages to close: NumberOfPagesToClose = %#v (present %v), want \"\"", v, ok)
	}
}
