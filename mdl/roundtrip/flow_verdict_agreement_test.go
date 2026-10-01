// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#876: `check -p` could not see that exec would refuse a statement
// under mdl 1. mxcli-rest's 14 scripts checked clean after
// `fmt --upgrade --header`, then exec refused three of them. diff already ran
// the splice plan (#850); check now runs the same verdict (decideFlowModify),
// so check, diff and exec agree on every `create or modify` of a stored flow.

// verdicts is what each of the three commands says about one script.
type verdicts struct {
	checkError   bool // check -p: an MDL-V1-REBUILD error
	checkWarning bool // check -p: the MDL-V1-REBUILD warning
	diffRefused  bool // diff: "Refused:" with the splice's refusal
	execRefused  bool // exec: the splice refusal, nothing written
	execRebuilt  bool // exec: the MDL-V1-REBUILD warning, the flow rebuilt
	checkMessage string
	execMessage  string
}

// verdictsOf runs check, diff and exec on script, in that order (the first two
// write nothing), and restores the working copy if exec wrote.
func (h *harness) verdictsOf(script string) verdicts {
	h.t.Helper()
	var v verdicts
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		h.t.Fatalf("parse: %v", errs[0])
	}
	for _, x := range h.exe.CheckFlowVerdicts(prog) {
		if x.RuleID != executor.FlowRebuildRule {
			continue
		}
		switch x.Severity {
		case linter.SeverityError:
			v.checkError = true
			v.checkMessage = x.Message
		case linter.SeverityWarning:
			v.checkWarning = true
		}
	}
	// diff runs exec on a scratch copy, so it reports whatever exec stops
	// at; the refusal compared here is the splice's.
	out := h.diff(script)
	v.diffRefused = strings.Contains(out, "Refused:") && strings.Contains(out, "cannot be spliced into the stored flow")
	before := h.snapshot()
	err := h.exec(script)
	changed := before.diff(h.snapshot())
	if err != nil && strings.Contains(err.Error(), "cannot be spliced into the stored flow") {
		v.execRefused = true
		v.execMessage = err.Error()
		if len(changed) != 0 {
			h.t.Errorf("a refused statement wrote %d unit(s)", len(changed))
		}
	}
	v.execRebuilt = strings.Contains(h.out.String(), "MDL-V1-REBUILD")
	if err != nil || len(changed) != 0 {
		h.restore()
	}
	return v
}

// agree fails the test where the three commands disagree under the script's
// version: under mdl 1, check's error, diff's Refused and exec's refusal; under
// mdl 0, check's warning and exec's rebuild (diff shows the rebuild as a
// modification, never as Refused). It returns whether exec refused or rebuilt.
func agree(t *testing.T, mdl1 bool, v verdicts) bool {
	t.Helper()
	if mdl1 {
		if v.checkError != v.diffRefused || v.diffRefused != v.execRefused {
			t.Errorf("mdl 1: check error=%v, diff refused=%v, exec refused=%v\n  exec: %s",
				v.checkError, v.diffRefused, v.execRefused, v.execMessage)
		}
		if v.checkWarning || v.execRebuilt {
			t.Errorf("mdl 1: a rebuild was reported (check warning=%v, exec rebuilt=%v)", v.checkWarning, v.execRebuilt)
		}
		if v.execRefused && !strings.Contains(v.checkMessage, v.execMessage) {
			t.Errorf("mdl 1: check does not quote exec's refusal\n  check: %s\n  exec:  %s", v.checkMessage, v.execMessage)
		}
		return v.execRefused
	}
	if v.checkWarning != v.execRebuilt {
		t.Errorf("mdl 0: check warning=%v, exec rebuilt=%v", v.checkWarning, v.execRebuilt)
	}
	if v.checkError || v.diffRefused || v.execRefused {
		t.Errorf("mdl 0: a refusal was reported (check error=%v, diff refused=%v, exec refused=%v)",
			v.checkError, v.diffRefused, v.execRefused)
	}
	return v.execRebuilt
}

// A statement added inside a stored loop's body: the splice does not edit inside
// a loop, so exec refuses it under mdl 1 and rebuilds under mdl 0. Before #876
// check passed it. (This test used the rehearsal's G1 guard-clause shape until
// #888 made the splice grow a guard.)
func TestFlowVerdictAgreement_ChangeInsideLoopBody(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const stub = `create or modify microflow MyFirstModule.Verdict_LoopBody ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
  end loop;
  return $Out;
end;
`
	const grown = `create or modify microflow MyFirstModule.Verdict_LoopBody ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
    set $Out = $Out + $U/Name;
  end loop;
  return $Out;
end;
`
	// Each verdict starts from the stored stub: exec of the grown flow under
	// mdl 0 rebuilds it, and verdictsOf restores the fixture after a write.
	stored := func() {
		if err := h.exec("mdl 1;\n" + stub); err != nil {
			t.Fatalf("create the stub: %v", err)
		}
	}
	// Control: the unchanged stub is unchanged everywhere.
	stored()
	if v := h.verdictsOf("mdl 1;\n" + stub); agree(t, true, v) || v.checkError {
		t.Errorf("the unchanged stub: %+v", v)
	}
	stored()
	if v := h.verdictsOf("mdl 1;\n" + grown); !agree(t, true, v) || !v.checkError {
		t.Errorf("mdl 1: a change inside a loop body is not refused by all three: %+v", v)
	}
	stored()
	if v := h.verdictsOf(grown); !agree(t, false, v) || !v.checkWarning {
		t.Errorf("mdl 0: a change inside a loop body is not a rebuild check warns about: %+v", v)
	}
}

// A statement whose flow an earlier statement of the script changes is not
// predicted from the stored flow: exec sees the earlier statement's result.
func TestFlowVerdictAgreement_EarlierStatementIsNotPredicted(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const stub = `create or modify microflow MyFirstModule.Verdict_Order ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
  end loop;
  return $Out;
end;
`
	const grown = `create or modify microflow MyFirstModule.Verdict_Order ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
    set $Out = $Out + $U/Name;
  end loop;
  return $Out;
end;
`
	if err := h.exec(stub); err != nil {
		t.Fatalf("create the stub: %v", err)
	}
	check := func(script string) bool {
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		return len(h.exe.CheckFlowVerdicts(prog)) > 0
	}
	// Control: alone, the grown flow is predicted (a refusal).
	if !check("mdl 1;\n" + grown) {
		t.Fatal("the grown flow alone is not predicted to be refused")
	}
	// After a drop it is a create, which exec makes.
	if check("mdl 1;\ndrop microflow MyFirstModule.Verdict_Order;\n" + grown) {
		t.Error("a create after a drop was predicted as a refused modify")
	}
	// Dropping the flow's module drops the flow with it: exec then creates it
	// (the create path makes the module), so a refusal would be a false error.
	if check("mdl 1;\ndrop module MyFirstModule;\n" + grown) {
		t.Error("a create after dropping its module was predicted as a refused modify")
	}
	// A folder moved to another module takes the flows in it along, so a
	// later statement may no longer name the stored flow.
	if check("mdl 1;\nmove folder MyFirstModule.Anything to OtherModule;\n" + grown) {
		t.Error("a statement after a folder left its module was predicted as a refused modify")
	}
}

// TestFlowVerdictAgreement_Corpus: over the Studio Pro-authored flows of PedApp
// and a subset of TestApp's, check, diff and exec reach the same verdict on
// each flow's description executed back (which splices: nothing to refuse) and
// on the description with a statement added inside its first loop body (which
// the splice cannot make: refused under mdl 1, rebuilt under mdl 0). The
// second kind is the control: a corpus with no refusal in it would make the
// agreement vacuous.
func TestFlowVerdictAgreement_Corpus(t *testing.T) {
	refusals := 0
	for _, c := range []struct {
		fx  fixture
		max int // flows with a loop to probe; 0 = all
	}{{pedApp, 0}, {testApp, 12}} {
		t.Run(c.fx.name, func(t *testing.T) {
			h := newFixtureHarness(t, c.fx)
			defer h.close()
			probed := 0
			for _, d := range h.documents() {
				if d.keyword != "microflow" && d.keyword != "nanoflow" {
					continue
				}
				if f, ok := c.fx.knownFailures[d.key()]; ok && len(f.laws) > 0 {
					continue // its description is the round trip's known failure
				}
				probe := strings.Contains(h.describeUnder("", d.target()), "end loop;") && (c.max == 0 || probed < c.max)
				if probe {
					probed++
				} else if c.max > 0 {
					continue // the TestApp subset: flows with a loop only
				}
				for _, header := range []string{"", "mdl 1;"} {
					mdl1 := header != ""
					described := h.describeUnder(header, d.target())
					if mdl1 {
						described = header + "\n" + described
					}
					t.Run(d.key()+map[bool]string{false: "/mdl 0", true: "/mdl 1"}[mdl1], func(t *testing.T) {
						agree(t, mdl1, h.verdictsOf(described))
						if !probe {
							return
						}
						i := strings.Index(described, "end loop;")
						withProbe := described[:i] + "declare $VerdictProbe Integer = 0;\n" + described[i:]
						if agree(t, mdl1, h.verdictsOf(withProbe)) {
							refusals++
						}
					})
				}
			}
			t.Logf("%s: %d flows probed inside a loop", c.fx.name, probed)
		})
	}
	if refusals == 0 {
		t.Error("no probe was refused or rebuilt: the agreement was never tested on a refusal")
	}
	t.Logf("%d probes refused or rebuilt", refusals)
}

// An alter exec refuses — here a fragment that returns, inserted before the
// stub's return — is refused under every version, and check reports it
// (MDL090) with exec's message. Control: an alter exec applies is not reported.
func TestFlowVerdictAgreement_Alter(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const stub = `create or modify microflow MyFirstModule.Verdict_Alter ($N: Integer)
returns Boolean as $Done
begin
  declare $Seen Integer = 0;
  return false;
end;
`
	const refused = `alter microflow MyFirstModule.Verdict_Alter {
  insert after $Seen begin return true; end;
};
`
	const applied = `alter microflow MyFirstModule.Verdict_Alter {
  insert after $Seen begin log info node 'Verdict' 'more'; end;
};
`
	checkOf := func(script string) []linter.Violation {
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		return h.exe.CheckFlowVerdicts(prog)
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		if err := h.exec("mdl 1;\n" + stub); err != nil {
			t.Fatalf("create the stub: %v", err)
		}
		got := checkOf(header + refused)
		err := h.exec(header + refused)
		if err == nil {
			t.Fatalf("%q: exec applied an alter whose fragment returns", header)
		}
		if !strings.Contains(err.Error(), "return") {
			t.Errorf("%q: exec refused the alter for another reason: %v", header, err)
		}
		if len(got) != 1 || got[0].RuleID != "MDL090" || got[0].Severity != linter.SeverityError ||
			!strings.Contains(got[0].Message, err.Error()) {
			t.Errorf("%q: check of the refused alter = %+v, want one MDL090 error quoting exec's %q", header, got, err)
		}
		if got := checkOf(header + applied); len(got) != 0 {
			t.Errorf("%q: check of an alter exec applies = %+v", header, got)
		}
		if err := h.exec(header + applied); err != nil {
			t.Errorf("%q: exec of the applied alter: %v", header, err)
		}
		h.restore()
	}
}
