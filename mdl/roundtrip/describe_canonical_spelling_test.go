// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestPedAppDescribeUsesCanonicalSpellings holds describe to R8 (#752) and
// R12: it never emits a registered deprecated spelling (`show_page`,
// `error_message`, `delete_behavior`, …), and every keyword it writes is
// already lowercase — `mxcli fmt` over describe output changes no letter.
func TestPedAppDescribeUsesCanonicalSpellings(t *testing.T) {
	describeUsesCanonicalSpellings(t, pedApp)
}

// TestTestAppDescribeUsesCanonicalSpellings is the same over TestApp, whose
// pages carry every page action (#759).
func TestTestAppDescribeUsesCanonicalSpellings(t *testing.T) {
	describeUsesCanonicalSpellings(t, testApp)
}

func describeUsesCanonicalSpellings(t *testing.T, fx fixture) {
	h := newFixtureHarness(t, fx)
	defer h.close()

	checked := 0
	for _, target := range describeTargets(h) {
		t.Run(target, func(t *testing.T) {
			out, err := h.describe(target)
			if err != nil || strings.TrimSpace(out) == "" {
				t.Skipf("describe %s: err=%v (judged by TestPedAppRoundTrip)", target, err)
			}
			prog, errs := visitor.Build(out)
			if len(errs) > 0 {
				t.Skipf("does not parse (judged by TestPedAppDescribeIsValidMdl1): %v", errs[0])
			}
			checked++
			for _, d := range prog.Deprecations {
				if !r8Codes[d.Code] {
					continue // the list-operation call forms are #737's (describe keeps them under mdl 0)
				}
				t.Errorf("line %d: describe emits the deprecated spelling %s\n--- describe output ---\n%s", d.Line, d.Code, out)
			}
			spans, ok := visitor.FormatSpans(out)
			if !ok {
				t.Fatalf("FormatSpans refused output that parses")
			}
			if lower := visitor.LowercaseKeywords(out, spans.Keywords); lower != out {
				t.Errorf("describe emits an upper-case keyword:\n%s", firstDifferentLine(out, lower))
			}
		})
	}
	if checked == 0 {
		t.Fatal("no describe output was checked — the test proves nothing")
	}
}

// r8Codes are the registry codes R8 (#752) owns, and R9's (#755): describe
// writes documentation as a doc comment, the folder as a clause and a workflow
// activity's caption as `caption`.
var r8Codes = map[string]bool{
	deprecation.PageActionWord:         true,
	deprecation.ErrorMessageKeyword:    true,
	deprecation.DeleteBehaviorClause:   true,
	deprecation.ReferenceSetUnderscore: true,
	deprecation.ReturnsNone:            true,
	deprecation.DocumentationClause:    true,
	deprecation.WorkflowCommentCaption: true,
	deprecation.FolderProperty:         true,
	deprecation.DocumentationProperty:  true,
}

func firstDifferentLine(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range al {
		if i < len(bl) && al[i] != bl[i] {
			return "  describe: " + al[i] + "\n  canonical: " + bl[i]
		}
	}
	return ""
}
