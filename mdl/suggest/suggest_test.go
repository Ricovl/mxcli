// SPDX-License-Identifier: Apache-2.0

package suggest

import "testing"

func TestClosest(t *testing.T) {
	known := []string{"Path", "Version", "ServiceName", "Password", "Enabled", "DisplayName"}
	for in, want := range map[string]string{
		"pathh":       "Path",        // prefix
		"Verison":     "Version",     // swapped neighbours
		"Passwd":      "Password",    // two deletions
		"Enabeld":     "Enabled",     // swapped neighbours
		"DisplayNmae": "DisplayName", // swapped neighbours
		"Colour":      "",            // nothing close
		"Xyz":         "",            // too short to guess past one edit
	} {
		if got := Closest(in, known); got != want {
			t.Errorf("Closest(%q) = %q, want %q", in, got, want)
		}
	}
}
