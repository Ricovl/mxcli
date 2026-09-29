// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// backslashLiterals returns the string literals of a script that hold a
// backslash: in an mdl 0 description, every string whose value has a line
// break, a tab or a backslash, which mdl 1 spells differently.
func backslashLiterals(src string) []string {
	lexer := parser.NewMDLLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	var out []string
	for _, tok := range lexer.GetAllTokens() {
		if tok.GetTokenType() != parser.MDLLexerSTRING_LITERAL {
			continue
		}
		if text := tok.GetText(); strings.Contains(text, `\`) {
			out = append(out, text)
		}
	}
	return out
}

// freshIDs matches the $ID of an element a write created: minted anew on
// every run, so not a difference between two runs.
var freshIDs = regexp.MustCompile(`"base64":"[^"]*"`)

// Every Studio Pro-authored document whose mdl 0 description spells a string
// with a backslash — an annotation ending in a line break, a caption over two
// lines, a JSON value in a change — round-trips under mdl 1 (ako/mxcli#804):
//
//   - describe in an `mdl 1;` script spells no string with a backslash escape;
//   - executing that description under the header writes nothing, and the
//     second describe equals the first;
//   - `fmt --upgrade --header` of the mdl 0 description is that same mdl 1
//     description.
//
// A document the round-trip allowlist already lists as losing something
// under mdl 0 must lose exactly that under mdl 1 too: the strings add nothing.
func TestStringLiteralsRoundTripUnderMdl1(t *testing.T) {
	for _, fx := range []fixture{pedApp, testApp} {
		t.Run(fx.name, func(t *testing.T) {
			h := newFixtureHarness(t, fx)
			defer h.close()
			found := 0
			for _, d := range h.documents() {
				plain, err := h.describe(d.target())
				if err != nil || len(backslashLiterals(plain)) == 0 {
					continue
				}
				found++
				t.Run(d.key(), func(t *testing.T) {
					first := h.describeUnder("mdl 1;", d.target())
					if lits := backslashLiterals(first); len(lits) > 0 {
						t.Errorf("describe under mdl 1 still spells %q", lits)
					}

					// Both are upgraded: describe still writes a few deprecated
					// spellings, which fmt rewrites alike in either.
					up, err := upgrade.Upgrade(plain, upgrade.Options{AddHeader: true})
					want, err1 := upgrade.Upgrade("mdl 1;\n"+first, upgrade.Options{AddHeader: true})
					if err != nil || err1 != nil {
						t.Errorf("fmt --upgrade --header: of the mdl 0 description: %v; of the mdl 1 one: %v", err, err1)
					} else if up.Source != want.Source {
						t.Errorf("fmt --upgrade --header of the mdl 0 description is not the mdl 1 description:\n%s",
							lineDiff(want.Source, up.Source))
					}

					_, known := fx.knownFailures[d.key()]
					var mdl0Wrote []string
					if known {
						_ = h.exec(plain)
						mdl0Wrote = h.orig.diff(h.snapshot())
						h.restore()
					}
					if err := h.exec("mdl 1;\n" + first); err != nil {
						t.Errorf("exec the mdl 1 description: %v\n%s", err, first)
					}
					wrote := h.orig.diff(h.snapshot())
					if len(wrote) > 0 {
						h.restore()
					}
					switch {
					case known && freshIDs.ReplaceAllString(strings.Join(wrote, "\n"), "") != freshIDs.ReplaceAllString(strings.Join(mdl0Wrote, "\n"), ""):
						t.Errorf("under mdl 1 the description writes other than under mdl 0:\nmdl 0:\n  %s\nmdl 1:\n  %s",
							strings.Join(mdl0Wrote, "\n  "), strings.Join(wrote, "\n  "))
					case !known && len(wrote) > 0:
						t.Errorf("the mdl 1 description wrote %d unit(s):\n  %s\n%s", len(wrote), strings.Join(wrote, "\n  "), first)
					case !known:
						if again := h.describeUnder("mdl 1;", d.target()); again != first {
							t.Errorf("describe -> exec -> describe under mdl 1 changed it:\n%s", lineDiff(first, again))
						}
					}
				})
			}
			// The fixtures hold such strings; finding none means the selection broke.
			if found == 0 {
				t.Fatalf("no document of %s describes a string with a backslash — the test looks at nothing", fx.name)
			}
			t.Logf("%d documents of %s describe a string with a backslash", found, fx.name)
		})
	}
}

// The reported case (ako/mxcli#804): TestApp's
// WorkflowCommons.ASU_UserTaskView_Migrate has a free annotation ending in a
// CR LF. Under mdl 1 describe writes the line break into the literal and the
// description writes nothing when executed. Control: the mdl 0 description,
// whose `\r\n` is a backslash, an r, a backslash and an n under mdl 1, states
// another annotation — the reported refusal — so the comparison can fail.
func TestAnnotationLineBreakUnderMdl1(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()
	const target, flow = "microflow WorkflowCommons.ASU_UserTaskView_Migrate", "ASU_UserTaskView_Migrate"

	first := h.describeUnder("mdl 1;", target)
	if !strings.Contains(first, "after startup flow.\r\n', position:") {
		t.Fatalf("describe under mdl 1 does not end the annotation in a CR LF:\n%s", first)
	}
	before := h.flowUnit(t, flow)
	if err := h.exec("mdl 1;\n" + first); err != nil {
		t.Fatalf("exec the mdl 1 description: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, flow), before) {
		t.Fatalf("the mdl 1 description of an unchanged flow wrote it:\n%s", h.out.String())
	}

	plain := h.mustDescribe(t, target)
	if !strings.Contains(plain, `after startup flow.\r\n', position:`) {
		t.Fatalf("the mdl 0 description does not escape the CR LF:\n%s", plain)
	}
	err := h.exec("mdl 1;\n" + plain)
	if err == nil && bytes.Equal(h.flowUnit(t, flow), before) {
		t.Fatal("the mdl 0 description executed under mdl 1 was taken as unchanged — the comparison cannot fail")
	}
	t.Logf("control: the mdl 0 description under mdl 1: %v", err)
}

// Neither fixture stores a backslash in a string, so one is written into a
// Studio Pro-authored flow (PedApp's VAL_Feedback, whose validation message
// ends in a line break): under mdl 1 describe writes it as itself, and the
// description round-trips. Under mdl 0 the same text is described `\\`.
func TestBackslashInStudioProFlowUnderMdl1(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const flow = "VAL_Feedback"

	first := h.describeUnder("mdl 1;", valFeedback)
	const anchor = "200 characters\n';"
	edited := strings.Replace(first, anchor, `200 characters in C:\temp\new, it''s`+anchor[len("200 characters"):], 1)
	if edited == first {
		t.Fatalf("the message has no %q — the fixture changed:\n%s", anchor, first)
	}
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec the edit: %v", err)
	}
	if len(h.orig.diff(h.snapshot())) == 0 {
		t.Fatal("the edit wrote nothing")
	}
	again := h.describeUnder("mdl 1;", valFeedback)
	if !strings.Contains(again, `200 characters in C:\temp\new, it''s`+"\n';") {
		t.Fatalf("describe under mdl 1 does not write the backslashes as themselves:\n%s", again)
	}
	if plain := h.mustDescribe(t, valFeedback); !strings.Contains(plain, `200 characters in C:\\temp\\new, it''s\n';`) {
		t.Errorf("the mdl 0 description does not escape the backslashes:\n%s", plain)
	}

	before := h.flowUnit(t, flow)
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("exec the mdl 1 description: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, flow), before) {
		t.Errorf("the mdl 1 description of the edited flow wrote it:\n%s", h.out.String())
	}
	if third := h.describeUnder("mdl 1;", valFeedback); third != again {
		t.Errorf("describe -> exec -> describe changed it:\n%s", lineDiff(again, third))
	}
}

// fmt --upgrade --header rewrites every escaped string literal so that the
// script builds the same model under mdl 1: in an expression the builder
// re-renders, an escaped line break makes the whole expression the one mdl 0
// stored (ako/mxcli#804). Both scripts run on a copy of PedApp and the models
// are compared. Control: the same script given only the header builds another.
func TestUpgradeOfEscapedStringsExecutesToTheSameModel(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()

	const script = `create module EscapeUpgrade;
create persistent entity EscapeUpgrade.Note (Body: String(unlimited), Code: String(20));
create enumeration EscapeUpgrade.Kind (Plain 'Line 1\nLine 2', Path 'C:\\temp\\new');
create microflow EscapeUpgrade.ACT_Write ($n: EscapeUpgrade.Note) begin
  @annotation 'Note\r\nwith a CR LF and C:\\temp'
  change $n (Body = '{\n  "a": 1\n}', Code = 'x');
  declare $s String = 'it''s\r\n' + TOSTRING( 1 ) + 'C:\\new';
  declare $t String = 'tab\there' + 'two\nlines' + 'three\nlines';
  log info node 'N' 'Line 1\nLine 2';
  commit $n;
end;
`
	res, err := upgrade.Upgrade(script, upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	errA, errB, diff := executeBoth(t, a, b, script, res.Source)
	if errA != nil || errB != nil {
		t.Fatalf("execute: %v / %v\n%s", errA, errB, res.Source)
	}
	if len(diff) > 0 {
		t.Errorf("the upgraded script builds another model:\n  %s\n%s", strings.Join(diff, "\n  "), res.Source)
	}

	if _, _, diff := executeBoth(t, a, b, script, "mdl 1;\n"+script); len(diff) == 0 {
		t.Fatal("the script under the header alone built the same model — the comparison cannot fail")
	}
}
