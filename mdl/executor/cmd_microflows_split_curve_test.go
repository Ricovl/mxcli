// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1414, second symptom ("layout drift on re-describe"):
// re-creating OpenAIConnector.EmbeddingsRequestMapping_POST from its own
// DESCRIBE lost three @curve lines — the one on the decision itself and the one
// on the first statement of each case branch. Measured on PedApp: the same
// loss on a plain `case $Color` and on an `if`'s own @curve; an `if` BRANCH kept
// its curve, because only that loop recorded it (applyPendingAnnotations). The
// split builders applied the statement's annotations with applyAnnotations,
// which sets caption and colour and never records the curve, so applyFlowCurves
// had nothing to stamp. (Not "@curve becoming @caption 'empty'" as the report
// guessed: the curve was dropped.)
func TestSplitCurvesSurviveDescribeExecDescribe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   []string
	}{
		{"case split and case branch", `create microflow M.F ($C: Enumeration(M.Color))
returns String
begin
  @curve(from: (0, -30), to: (0, 15))
  case $C
    when Red then
      @curve(from: (30, 0), to: (-15, 0))
      log info 'a';
      return 'r';
    when (empty) then
      return 'e';
  end case;
end;`, []string{"@curve(from: (0, -30), to: (0, 15))", "@curve(from: (30, 0), to: (-15, 0))"}},
		{"if split", `create microflow M.G ($B: Boolean)
returns String
begin
  @curve(from: (0, -30), to: (0, 15))
  if $B then
    return 'x';
  else
    return 'y';
  end if;
end;`, []string{"@curve(from: (0, -30), to: (0, 15))"}},
		{"type split branch", `create microflow M.H ($A: M.Animal)
returns String
begin
  split type $A
    when M.Dog then
      @curve(from: (30, 0), to: (-15, 0))
      log info 'dog';
      return 'd';
    when (empty) then
      return 'e';
  end split;
end;`, []string{"@curve(from: (30, 0), to: (-15, 0))"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.script)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
			fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
				measurer:     &layoutMeasurer{varTypes: map[string]string{}},
				varTypes:     map[string]string{"C": "Enumeration(M.Color)", "A": "M.Animal"},
				declaredVars: map[string]string{"C": "Enumeration(M.Color)", "B": "Boolean", "A": "M.Animal"}}
			oc := fb.buildFlowGraph(body, nil)
			out := strings.Join(formatMicroflowActivities(newTestExecutor().newExecContext(t.Context()), &microflows.Microflow{ObjectCollection: oc}, nil, nil), "\n")
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("%s lost after exec:\n%s", want, out)
				}
			}
		})
	}
}

// The other line of #1414's drift: `@anchor(from: top, …)` on the "Not
// Snowflake?" if. That `from:` is the side its closing merge is LEFT on
// (mergeExitAnchor, #767), but the merge is `merge shared1;`, declared on its
// own, so the rebuilt if has no closing merge to apply it to and the exit came
// back from the right. The declaration owns that edge and now carries the side.
func TestMergeDeclarationCarriesItsExitSide(t *testing.T) {
	merge := &microflows.ExclusiveMerge{}
	merge.ID = "m"
	for _, tc := range []struct {
		side int
		want string
	}{
		{AnchorTop, "@anchor(from: top)"},
		{AnchorBottom, "@anchor(from: bottom)"},
		{AnchorRight, ""}, // the default stays silent
	} {
		flows := map[model.ID][]*microflows.SequenceFlow{"m": {{OriginID: "m", DestinationID: "x", OriginConnectionIndex: tc.side}}}
		got := strings.Join(mergeDeclarationLines(1, "shared1", merge, nil, flows), "\n")
		if tc.want == "" && strings.Contains(got, "@anchor") || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("side %d: got %q, want %q", tc.side, got, tc.want)
		}
		if !strings.HasSuffix(got, "merge shared1;") {
			t.Errorf("declaration must end with the statement: %q", got)
		}
	}
}
