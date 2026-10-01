// SPDX-License-Identifier: Apache-2.0

package langver

import (
	"strings"
	"testing"
)

// A language change's warning ends by pointing at `mxcli help <code>`, which
// prints the change's entry (ako/mxcli#714 decision 3).
func TestChangeWarningEndsWithHelpHint(t *testing.T) {
	c := Change{Code: "MDL-V1-EXAMPLE", Since: V1, Old: "x is y", New: "z"}
	got := c.Warning(V0)
	if want := "(mxcli help MDL-V1-EXAMPLE)"; !strings.HasSuffix(got, want) {
		t.Fatalf("Warning = %q, want it to end with %q", got, want)
	}
	if !strings.Contains(got, "x is y under mdl 0; under mdl 1 it means z.") {
		t.Errorf("Warning lost its message: %q", got)
	}
}
