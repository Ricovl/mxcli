// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// `alter layout … set (RenderMode: 'Main') on ctnMain` was refused with
// "property \"RenderMode\" is not a property of this built-in widget", although
// CREATE writes Forms$DivContainer.RenderMode (mendixlabs/mxcli#1582).
func TestSetWidgetProperty_ContainerRenderMode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"Main", "Main"}, {"nav", "Nav"}, {"Div", "Div"}} {
		rawData := makeRawPage(bson.D{
			{Key: "$Type", Value: "Forms$DivContainer"},
			{Key: "Name", Value: "ctnMain"},
			{Key: "RenderMode", Value: "Div"},
		})
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("ctnMain", "RenderMode", tc.in); err != nil {
			t.Fatalf("SetWidgetProperty(RenderMode=%s): %v", tc.in, err)
		}
		if got := bsonnav.DGetString(findBsonWidget(rawData, "ctnMain").widget, "RenderMode"); got != tc.want {
			t.Errorf("RenderMode = %q, want %q", got, tc.want)
		}
	}
}

func TestSetWidgetProperty_ContainerRenderModeInvalid(t *testing.T) {
	rawData := makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "ctnMain"},
		{Key: "RenderMode", Value: "Main"},
	})
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("ctnMain", "RenderMode", "H1"); err == nil {
		t.Fatal("RenderMode H1 on a container must be refused")
	}
	if got := bsonnav.DGetString(findBsonWidget(rawData, "ctnMain").widget, "RenderMode"); got != "Main" {
		t.Errorf("a refused value changed the stored RenderMode to %q", got)
	}
}
