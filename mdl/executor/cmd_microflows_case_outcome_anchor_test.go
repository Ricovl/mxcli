// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A CASE stored each outcome's side by WHEN order alone — 1st top, 2nd right,
// 3rd bottom — and MDL had no way to say otherwise (mendixlabs/mxcli#1587,
// ako/mxcli#1492): `@anchor(A: (from: right))` was refused with MDL092 and
// `@anchor(from: right)` on the CASE was accepted and ignored. The WHEN-order
// default stays for up to three cases — it is how DESCRIBE reads the case order
// back (TestEnumSplitRoundtripPreservesEmptyCaseBeforeNamedCases) — so the
// per-outcome form is how a script, and a description, says otherwise.

const caseOutcomeMDL = `
  case $E
    when A then
      log info node 'T' 'a';
    when B then
      log info node 'T' 'b';
    when (empty) then
      log info node 'T' 'empty';
  end case;
  log info node 'T' 'after';`

// caseOutcomeSides returns, per case value, the sides of the flow leaving the split.
func caseOutcomeSides(t *testing.T, fb *flowBuilder) map[string][2]int {
	t.Helper()
	var splitID model.ID
	for _, o := range fb.objects {
		if _, ok := o.(*microflows.ExclusiveSplit); ok {
			splitID = o.GetID()
		}
	}
	out := map[string][2]int{}
	for _, f := range fb.flows {
		if f.OriginID != splitID {
			continue
		}
		if v, ok := enumCaseValue(f); ok {
			out[v] = [2]int{f.OriginConnectionIndex, f.DestinationConnectionIndex}
		}
	}
	if len(out) == 0 {
		t.Fatal("no case flows built")
	}
	return out
}

func TestCaseOutcomeAnchorChoosesEachOutcomesSide(t *testing.T) {
	fb := buildFlowFromMDL(t, "  @anchor(A: (from: right), (empty): (from: bottom, to: top))"+caseOutcomeMDL)
	if len(fb.errors) > 0 {
		t.Fatalf("build errors: %v", fb.errors)
	}
	got := caseOutcomeSides(t, fb)
	if got["A"][0] != AnchorRight {
		t.Errorf("A leaves side %d, want right (%d) as written", got["A"][0], AnchorRight)
	}
	if got["(empty)"] != [2]int{AnchorBottom, AnchorTop} {
		t.Errorf("(empty) runs %d -> %d, want bottom -> top (%d -> %d) as written",
			got["(empty)"][0], got["(empty)"][1], AnchorBottom, AnchorTop)
	}
	// An outcome the annotation does not name keeps its default.
	if got["B"][0] != AnchorRight {
		t.Errorf("B leaves side %d, want its default right (%d)", got["B"][0], AnchorRight)
	}
}

// A keyword as an enumeration value is still a key: `Open` is a lexer keyword,
// and a keyword key used to fall through to the positional form and be refused.
func TestCaseOutcomeAnchorAcceptsAKeywordValue(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  @anchor(Open: (from: bottom))
  case $State
    when Open then log info node 'T' 'o';
    when Closed then log info node 'T' 'c';
  end case;`)
	if got := caseOutcomeSides(t, fb); got["Open"][0] != AnchorBottom {
		t.Errorf("Open leaves side %d, want bottom (%d) as written", got["Open"][0], AnchorBottom)
	}
}

// `@anchor(from: …)` on a CASE is the flow leaving it — out of its closing
// merge — exactly as on an IF (#767). It was accepted and dropped.
func TestCaseFromAnchorIsTheFlowLeavingItsMerge(t *testing.T) {
	fb := buildFlowFromMDL(t, "  @anchor(from: bottom)"+caseOutcomeMDL)
	var merge model.ID
	for _, o := range fb.objects {
		if _, ok := o.(*microflows.ExclusiveMerge); ok {
			merge = o.GetID()
		}
	}
	for _, f := range fb.flows {
		if f.OriginID == merge {
			if f.OriginConnectionIndex != AnchorBottom {
				t.Errorf("the flow leaving the CASE's merge starts at side %d, want bottom (%d)", f.OriginConnectionIndex, AnchorBottom)
			}
			return
		}
	}
	t.Fatal("no flow leaves the CASE's merge")
}

// DESCRIBE writes an outcome's side only where it differs from what the
// builder would choose, so a description rebuilds the same sides and an
// ordinary CASE gains no annotation.
func TestDescribeCaseOutcomeAnchorsRoundTrip(t *testing.T) {
	const src = `create microflow M.Case ($E: M.Kind) returns Boolean
begin
  @anchor(A: (from: right), (empty): (from: bottom, to: top))
  case $E
    when A then
      log info node 'T' 'a';
    when B then
      log info node 'T' 'b';
    when (empty) then
      log info node 'T' 'empty';
  end case;
  return true;
end;`
	out, oc := describeBuilt(t, src)
	// The (empty) outcome's `to: top` lands on its first activity, which says
	// so itself as `@anchor(to: top)`; the CASE only carries what no statement
	// can. Its `from: bottom` is the third outcome's default, so it is not
	// restated, and neither is anything about B.
	for _, want := range []string{"A: (from: right)", "@anchor(to: top)"} {
		if !strings.Contains(out, want) {
			t.Errorf("description lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"B: (", "(empty): ("} {
		if strings.Contains(out, unwanted) {
			t.Errorf("description restates a default with %q:\n%s", unwanted, out)
		}
	}
	again, _ := describeBuilt(t, "create microflow M.Case ($E: M.Kind) returns Boolean\nbegin\n"+out+"\nend;")
	if again != out {
		t.Errorf("not a fixed point:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
	_ = oc

	plain, _ := describeBuilt(t, "create microflow M.Case ($E: M.Kind) returns Boolean\nbegin\n"+caseOutcomeMDL+"\n  return true;\nend;")
	if strings.Contains(plain, "@anchor(") {
		t.Errorf("an unannotated CASE is described with an anchor:\n%s", plain)
	}
}

func TestCaseOutcomeAnchorValidation(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unknown value", "  @anchor(Z: (from: right))" + caseOutcomeMDL, "Z"},
		{"true: on a CASE", "  @anchor(true: (from: right))" + caseOutcomeMDL, "true"},
		{"value key on an IF", `
  @anchor(A: (from: right))
  if $F then log info node 'T' 'x'; end if;`, "A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var found bool
			for _, v := range checkMicroflowSource(t, "create microflow M.F ($E: M.Kind, $F: Boolean)\nbegin\n"+tc.src+"\nend;") {
				if v.RuleID == invalidAnchorRule && strings.Contains(v.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no %s refusal naming %q", invalidAnchorRule, tc.want)
			}
		})
	}
}

// The CASE's own `from:` round-trips like an IF's: describe reads it off the
// flow leaving the merge.
func TestDescribeCaseFromAnchorRoundTrips(t *testing.T) {
	src := "create microflow M.Case ($E: M.Kind) returns Boolean\nbegin\n  @anchor(from: bottom)" + caseOutcomeMDL + "\n  return true;\nend;"
	out, _ := describeBuilt(t, src)
	if !strings.Contains(out, "from: bottom") {
		t.Errorf("the CASE's from: is not described:\n%s", out)
	}
	again, _ := describeBuilt(t, "create microflow M.Case ($E: M.Kind) returns Boolean\nbegin\n"+out+"\nend;")
	if again != out {
		t.Errorf("not a fixed point:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}
