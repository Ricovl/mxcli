// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1756: APP TITLE / APP ICON reach the backend as spec fields this
// backend does not author over PED. It refuses them before reading or writing
// anything, rather than reporting the profile updated without them.
func TestUpdateNavigationProfile_AppTitleAndIconRefused(t *testing.T) {
	title, icon := "EWO", "MyModule.Images.Logo"
	for name, spec := range map[string]types.NavigationProfileSpec{
		"app title": {AppTitle: &title},
		"app icon":  {AppIcon: &icon},
	} {
		f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
		b := &Backend{client: f.connectClient(t)}
		err := b.UpdateNavigationProfile("", "Responsive", spec)
		if err == nil || !strings.Contains(err.Error(), "app title and app icon") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
		for _, c := range f.calls {
			if strings.HasPrefix(c.Name, "ped_") {
				t.Errorf("%s: %s was called before the refusal", name, c.Name)
			}
		}
	}
}
