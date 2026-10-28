// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// `alter layout … { set (LayoutMode: 'Sidebar') on layoutContainer; }` — a
// scroll container's Headline / Sidebar, which CREATE LAYOUT now writes
// (ako/mxcli#1703). It fell to the pluggable path and was refused there.
func scrollContainerPage(withMode bool) bson.D {
	w := bson.D{
		{Key: "$Type", Value: "Forms$ScrollContainer"},
		{Key: "Name", Value: "layoutContainer"},
	}
	if withMode {
		w = append(w, bson.E{Key: "LayoutMode", Value: "Headline"})
	}
	return makeRawPage(w)
}

func TestSetWidgetProperty_ScrollContainerLayoutMode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"Sidebar", "Sidebar"}, {"sidebar", "Sidebar"}, {"Headline", "Headline"}} {
		rawData := scrollContainerPage(true)
		m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("layoutContainer", "LayoutMode", tc.in); err != nil {
			t.Fatalf("SetWidgetProperty(LayoutMode=%s): %v", tc.in, err)
		}
		if got := bsonnav.DGetString(findBsonWidget(rawData, "layoutContainer").widget, "LayoutMode"); got != tc.want {
			t.Errorf("LayoutMode = %q, want %q", got, tc.want)
		}
	}
}

func TestSetWidgetProperty_ScrollContainerLayoutModeRefusals(t *testing.T) {
	// A value outside the enumeration, and the stored value is left alone.
	rawData := scrollContainerPage(true)
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("layoutContainer", "LayoutMode", "Topbar"); err == nil ||
		!strings.Contains(err.Error(), "Headline or Sidebar") {
		t.Errorf("LayoutMode Topbar: err = %v, want a refusal naming Headline or Sidebar", err)
	}
	if got := bsonnav.DGetString(findBsonWidget(rawData, "layoutContainer").widget, "LayoutMode"); got != "Headline" {
		t.Errorf("a refused value changed the stored LayoutMode to %q", got)
	}

	// A document without the key: never invent one (overlay writes).
	rawData = scrollContainerPage(false)
	m = &Mutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("layoutContainer", "LayoutMode", "Sidebar"); err == nil {
		t.Error("a scroll container that stores no LayoutMode must not have one invented")
	}
	if bsonnav.DGet(findBsonWidget(rawData, "layoutContainer").widget, "LayoutMode") != nil {
		t.Error("the refused write added a LayoutMode key")
	}
}
