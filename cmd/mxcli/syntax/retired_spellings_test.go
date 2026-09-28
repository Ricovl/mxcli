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
		// The rest of #755: a listing is `list` (MDL-DEPR002), the entity
		// summary is gone (MDL-V1-SHOWSUMMARY), R10's names.
		"show modules":        "list modules (MDL-DEPR002)",
		"show entities":       "list entities (MDL-DEPR002)",
		"show microflows":     "list microflows (MDL-DEPR002)",
		"show pages":          "list pages (MDL-DEPR002)",
		"show widgets":        "list widgets (MDL-DEPR002)",
		"show callers of":     "list callers of (MDL-DEPR002)",
		"show impact of":      "list impact of (MDL-DEPR002)",
		"show references to":  "list references to (MDL-DEPR002)",
		"show access on":      "list access on (MDL-DEPR002)",
		"show navigation":     "list navigation (MDL-DEPR002)",
		"show settings":       "list settings (MDL-DEPR002)",
		"show catalog tables": "list catalog tables (MDL-DEPR002)",
		"show entity module.": "describe entity (MDL-V1-SHOWSUMMARY)",
		"image collection;":   "list image collections (MDL-DEPR130)",
		"create model ":       "create ai model (MDL-DEPR131)",
		"drop model ":         "drop ai model (MDL-DEPR131)",
		"list models":         "list ai models (MDL-DEPR131)",
		"security level ":     "alter app security ( SecurityLevel: … ) (MDL-DEPR133)",
		"security demo users": "alter app security ( EnableDemoUsers: … ) (MDL-DEPR133)",
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
