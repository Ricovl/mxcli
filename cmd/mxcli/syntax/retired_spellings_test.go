// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// `mxcli syntax` is the reference an agent reads before writing MDL, and nothing
// checks it against the parser — so a spelling the parser has dropped can sit in
// it indefinitely. Two did, and both cost a build round-trip to discover
// (mxcli-todo findings #8).
//
// This pins the corrections. It is a spelling guard, not a parse: the snippets
// are fragments (a DATAVIEW body, a property line) that do not stand alone as
// statements, so they cannot simply be fed to the parser.
func TestSyntaxDocs_NoRetiredSpellings(t *testing.T) {
	retired := []struct {
		text   string
		reason string
	}{
		{
			"Binds:",
			"the parser rejects it: \"'Binds:' is no longer supported, use 'Attribute:' instead\"",
		},
		{
			"MICROFLOW Module.MF()",
			"a zero-argument microflow DATASOURCE takes no parentheses (unlike RETRIEVE/CALL, where they are normal)",
		},
	}

	for _, f := range All() {
		for _, r := range retired {
			for field, text := range map[string]string{"Syntax": f.Syntax, "Example": f.Example} {
				if strings.Contains(text, r.text) {
					t.Errorf("syntax topic %q, %s field, still shows %q — %s", f.Path, field, r.text, r.reason)
				}
			}
		}
	}
}

// R6 (ako/mxcli#755): the old verbs still parse as deprecated aliases, but the
// reference an agent reads shows only the canonical ones. Case-insensitive,
// because the entries mix the two cases.
func TestSyntaxDocs_NoR6DeprecatedVerbs(t *testing.T) {
	deprecated := map[string]string{
		"rest call ":            "call rest service (MDL-DEPR094)",
		"define fragment":       "create fragment (MDL-DEPR096)",
		"remove module roles":   "drop module roles (MDL-DEPR091)",
		"show project security": "describe app security (MDL-DEPR090)",
		"show security matrix":  "describe security matrix (MDL-DEPR090)",
		"show structure":        "describe structure (MDL-DEPR090)",
		"show context of":       "describe context of (MDL-DEPR090)",
		"language remove '":     "language drop (MDL-DEPR092)",
		"remove group '":        "drop group (MDL-DEPR092)",
	}
	for _, f := range All() {
		for field, text := range map[string]string{"Syntax": f.Syntax, "Example": f.Example} {
			low := strings.ToLower(text)
			for old, canonical := range deprecated {
				if strings.Contains(low, old) {
					t.Errorf("syntax topic %q, %s field, shows %q — write %s", f.Path, field, old, canonical)
				}
			}
		}
	}
}
