// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1220 (ako/mxcli#1285): copying Atlas_Core.Atlas_Default with
// `describe layout` → `create layout` lost the responsive sidebar toggle, which
// describe announced as
//
//	-- Forms$SidebarToggleButton (sidebarToggle3)  -- NOT re-executable: mxcli
//	cannot author this widget, so re-running this script would drop it
//
// and Atlas's slide-in navigation on phones and tablets depends on it.

// storedSidebarToggle is the widget exactly as Atlas_Core.Atlas_Default stores
// it (PedApp, 11.13.0), less its design properties.
func storedSidebarToggle(renderType string) map[string]any {
	return map[string]any{
		"$Type": "Forms$SidebarToggleButton",
		"Name":  "sidebarToggle3",
		"Appearance": map[string]any{
			"$Type":          "Forms$Appearance",
			"Class":          "toggle-btn",
			"Style":          "",
			"DynamicClasses": "",
		},
		"ButtonStyle": "Primary",
		"CaptionTemplate": map[string]any{
			"$Type": "Forms$ClientTemplate",
			"Template": map[string]any{
				"$Type": "Texts$Text",
				"Items": []any{int32(3), map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": ""}},
			},
			"Parameters": []any{int32(2)},
		},
		"Icon": map[string]any{
			"$Type": "Forms$IconCollectionIcon",
			"Image": "Atlas_Core.Atlas_Filled.navigation-menu",
		},
		"RenderType": renderType,
		"TabIndex":   int32(0),
		"Tooltip": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{int32(3), map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Toggle Menu"}},
		},
	}
}

func TestDescribeSidebarToggle_RoundTrips(t *testing.T) {
	got := describeStoredWidget(t, storedSidebarToggle("Button"))
	if strings.Contains(got, "NOT re-executable") {
		t.Fatalf("sidebar toggle still described as unauthorable:\n%s", got)
	}
	for _, want := range []string{
		"sidebartoggle sidebarToggle3", "Tooltip: 'Toggle Menu'", "ButtonStyle: Primary",
		"Icon: 'Atlas_Core.Atlas_Filled.navigation-menu'", "Class: 'toggle-btn'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("description lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "RenderMode") {
		t.Errorf("Button is the default render mode and is not said:\n%s", got)
	}

	tb, ok := rebuildMenuWidget(t, got).(*pages.SidebarToggleButton)
	if !ok {
		t.Fatalf("replay did not build a sidebar toggle from:\n%s", got)
	}
	if tb.Name != "sidebarToggle3" || tb.Class != "toggle-btn" {
		t.Errorf("replayed name/class = %q/%q", tb.Name, tb.Class)
	}
	if tb.ButtonStyle != pages.ButtonStylePrimary {
		t.Errorf("replayed ButtonStyle = %q, want Primary", tb.ButtonStyle)
	}
	if tb.RenderMode != pages.ButtonRenderModeButton {
		t.Errorf("replayed RenderMode = %q, want Button", tb.RenderMode)
	}
	if tb.Icon == nil || tb.Icon.Image != "Atlas_Core.Atlas_Filled.navigation-menu" {
		t.Errorf("replayed Icon = %+v, want the Atlas_Filled navigation-menu icon", tb.Icon)
	}
	if tb.Tooltip == nil || !strings.Contains(strings.Join(mapValues(tb.Tooltip.Translations), ""), "Toggle Menu") {
		t.Errorf("replayed Tooltip = %+v, want 'Toggle Menu'", tb.Tooltip)
	}
}

// Link is the non-default render mode, so it is the one a describer that
// forgets the property loses.
func TestDescribeSidebarToggle_RoundTripsLinkRenderMode(t *testing.T) {
	got := describeStoredWidget(t, storedSidebarToggle("Link"))
	tb, ok := rebuildMenuWidget(t, got).(*pages.SidebarToggleButton)
	if !ok {
		t.Fatalf("replay did not build a sidebar toggle from:\n%s", got)
	}
	if tb.RenderMode != pages.ButtonRenderModeLink {
		t.Errorf("replayed RenderMode = %q, want Link; described as:\n%s", tb.RenderMode, got)
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
