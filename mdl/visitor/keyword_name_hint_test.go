// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1533 (1): a workflow activity named with an MDL keyword —
// `call microflow M.ACT_Close as close` — gave a bare "mismatched input 'close'
// expecting {IDENTIFIER, QUOTED_IDENTIFIER}". The keyword hint fired only for a
// fixed list of 41 words, and `close` was not on it.
package visitor

import (
	"strings"
	"testing"
)

const activityNamedSlot = "create workflow M.W\nbegin\n  call microflow M.ACT_Close as %s;\nend workflow;"

func TestKeywordUsedAsNameIsHinted(t *testing.T) {
	_, errs := Build(strings.Replace(activityNamedSlot, "%s", "close", 1))
	got := errsText(errs)
	if !strings.Contains(got, "mismatched input 'close'") {
		t.Fatalf("expected the parse error on 'close', got:\n%s", got)
	}
	for _, want := range []string{"'close' is a keyword in MDL", `"close"`} {
		if !strings.Contains(got, want) {
			t.Errorf("hint does not mention %q:\n%s", want, got)
		}
	}
}

// The control that makes the advice honest: the quoted form must parse.
func TestKeywordUsedAsNameQuotedParses(t *testing.T) {
	if _, errs := Build(strings.Replace(activityNamedSlot, "%s", `"close"`, 1)); len(errs) != 0 {
		t.Fatalf("the hint recommends quoting, but the quoted form does not parse: %v", errs)
	}
}

// A word on the old fixed list must still get the hint, and exactly once — the
// generic path and the list path must not both fire.
func TestKeywordHintNotDoubled(t *testing.T) {
	for _, name := range []string{"Title", "close"} {
		got := hintFor(t, name)
		if n := strings.Count(got, "Run 'mxcli syntax keywords'"); n != 1 {
			t.Errorf("%s: keyword hint appears %d times, want 1:\n%s", name, n, got)
		}
	}
}

// A token that is not identifier-shaped gets no keyword hint.
func TestKeywordHintNotForPunctuation(t *testing.T) {
	got := enhanceErrorMessage("mismatched input ';' expecting {IDENTIFIER, QUOTED_IDENTIFIER}", "")
	if strings.Contains(got, "is a keyword in MDL") {
		t.Errorf("punctuation was hinted as a keyword:\n%s", got)
	}
}
