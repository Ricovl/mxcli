// SPDX-License-Identifier: Apache-2.0

package javaactions

import "testing"

// The marker search folds case, and strings.ToLower does not keep byte
// lengths: "İ" (U+0130, 2 bytes) lowers to "i̇" (3 bytes) and the Kelvin sign
// (3 bytes) to "k" (1 byte). An index found in the lowered copy then slices the
// original at the wrong offset, so one such character in a comment above the
// markers — a Turkish name in an action's documentation — shifted the retained
// EXTRA CODE by a byte, and the rewrite wrote Java that does not compile.
func TestRetainedSectionsSurvivesCaseFoldingThatChangesByteLength(t *testing.T) {
	for _, pre := range []string{"İstanbul", "Kelvin", "plain"} {
		src := "// " + pre + "\npublic class A extends UserAction<Boolean>\n{\n" +
			"\t// BEGIN EXTRA CODE\n\tprivate int helper() { return 1; }\n\t// END EXTRA CODE\n}\n"
		_, extra := RetainedSections(src)
		if want := "private int helper() { return 1; }"; extra != want {
			t.Errorf("after %q: extra code = %q, want %q", pre, extra, want)
		}
	}
}
