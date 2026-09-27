// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// See sdk/javaactions TestRetainedSectionsSurvivesCaseFoldingThatChangesByteLength:
// an index into strings.ToLower(s) is not an index into s once a character
// changes byte length when lowered, and DESCRIBE reads Java and JavaScript
// action bodies through this function.
func TestSliceBetweenFoldKeepsByteOffsets(t *testing.T) {
	for _, pre := range []string{"İstanbul", "Kelvin", "plain"} {
		src := "// " + pre + "\n// BEGIN USER CODE\nreturn true;\n// END USER CODE\n"
		got, ok := sliceBetweenFold(src, "// BEGIN USER CODE", "// END USER CODE")
		if !ok || got != "\nreturn true;\n" {
			t.Errorf("after %q: got %q, %v", pre, got, ok)
		}
	}
}
