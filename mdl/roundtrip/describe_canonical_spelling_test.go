// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestPedAppDescribeUsesCanonicalSpellings holds describe to R8 (#752) and
// R12: it never emits a registered deprecated spelling (`show_page`,
// `error_message`, `delete_behavior`, `count($L)`, …) in either language — the
// default mdl 1 and `describe --mdl 0` (ako/mxcli#840) — and every keyword it
// writes is already lowercase: `mxcli fmt` over describe output changes no
// letter.
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
			for _, v := range []langver.Version{langver.V1, langver.V0} {
				out, err := h.describeAs(v, target)
				if err != nil || strings.TrimSpace(out) == "" {
					t.Skipf("describe %s: err=%v (judged by TestPedAppRoundTrip)", target, err)
				}
				prog, errs := visitor.Build(out)
				if len(errs) > 0 {
					t.Skipf("does not parse (judged by TestPedAppDescribeIsValidMdl1): %v", errs[0])
				}
				if got := prog.LanguageVersion; got != v {
					t.Fatalf("describe in %v reads back as %v: its header is missing or wrong", v, got)
				}
				checked++
				for _, d := range prog.Deprecations {
					t.Errorf("%v, line %d: describe emits the deprecated spelling %s\n--- describe output ---\n%s", v, d.Line, d.Code, out)
				}
				spans, ok := visitor.FormatSpans(out)
				if !ok {
					t.Fatalf("FormatSpans refused output that parses")
				}
				if lower := visitor.LowercaseKeywords(out, spans.Keywords); lower != out {
					t.Errorf("%v: describe emits an upper-case keyword:\n%s", v, firstDifferentLine(out, lower))
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("no describe output was checked — the test proves nothing")
	}
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
