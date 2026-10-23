// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

func TestSetWebUISettings_UpdatesStoredProperty(t *testing.T) {
	raw := map[string]any{
		"$Type":              "Forms$WebUIProjectSettingsPart",
		"UseOptimizedClient": "Yes",
		"UrlPrefix":          "p",
	}
	SetWebUISettings(&model.WebUISettings{UseOptimizedClient: "No", UrlPrefix: "changed"}, raw)
	if raw["UseOptimizedClient"] != "No" {
		t.Errorf("UseOptimizedClient = %#v, want No", raw["UseOptimizedClient"])
	}
	// MDL does not author the other Web UI properties; they pass through.
	if raw["UrlPrefix"] != "p" {
		t.Errorf("UrlPrefix = %#v, want the stored p", raw["UrlPrefix"])
	}
}

// UseOptimizedClient arrived in Mendix 9.10; a part from before it must not gain
// the property — the mendixlabs/mxcli#759 shape, a key the type does not define.
func TestSetWebUISettings_DoesNotIntroduceAbsentProperty(t *testing.T) {
	raw := map[string]any{"$Type": "Forms$WebUIProjectSettingsPart"}
	SetWebUISettings(&model.WebUISettings{UseOptimizedClient: "Yes"}, raw)
	if _, ok := raw["UseOptimizedClient"]; ok {
		t.Errorf("UseOptimizedClient introduced onto a part that did not store it: %#v", raw)
	}
}

func TestIsWebUIPart(t *testing.T) {
	for typeName, want := range map[string]bool{
		"Forms$WebUIProjectSettingsPart":    true,
		"Settings$WebUIProjectSettingsPart": true,
		"Settings$ModelSettings":            false,
	} {
		if got := IsWebUIPart(typeName); got != want {
			t.Errorf("IsWebUIPart(%q) = %t, want %t", typeName, got, want)
		}
	}
}
