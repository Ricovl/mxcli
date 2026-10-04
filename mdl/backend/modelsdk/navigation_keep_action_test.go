// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#980: a stored action the script could not state is written back
// verbatim. Without KeepAction the writer had only page / microflow / sign out
// to choose from and stored Forms$NoAction — a nanoflow menu item, TestApp's
// 'Item 4', came back dead.
func TestNavMenuAction_WritesAKeptActionVerbatim(t *testing.T) {
	stored, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: "keep-me"},
		{Key: "$Type", Value: "Forms$CallNanoflowClientAction"},
		{Key: "Nanoflow", Value: "MyFirstModule.Nanoflow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := navMenuAction(types.NavMenuItemSpec{Caption: "Item 4", KeepAction: stored})
	if navGetString(got, "$Type") != "Forms$CallNanoflowClientAction" || navGetString(got, "Nanoflow") != "MyFirstModule.Nanoflow" {
		t.Fatalf("kept action not written verbatim: %v", got)
	}
	// CONTROL: the same item without a kept action is the writer's NoAction —
	// the loss this guards against, so the case above is not passing by default.
	if got := navMenuAction(types.NavMenuItemSpec{Caption: "Item 4"}); navGetString(got, "$Type") != "Forms$NoAction" {
		t.Fatalf("control: an item with no action wrote %v", got)
	}
}
