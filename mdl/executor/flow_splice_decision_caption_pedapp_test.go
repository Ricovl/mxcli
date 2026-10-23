// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1610 (ako/mxcli#1538), and item 1 of saingge's comment on
// mendixlabs/mxcli#1517: under `mdl 1`, re-running `create or modify` with
// only an `@caption` added to an existing `if` was refused —
//
//	exec refuses with MDL-V1-REBUILD: 'the If at (360, 200) changes where its
//	paths end or meet — a branch's return, or its else, is added or taken
//	away'. Nothing about the paths changed.
//
// A decision's caption is a property of the decision, like its condition: it
// is set in place, as a changed condition is (ako/mxcli#888), and the decision
// keeps its $ID, its flows and both of its paths.

const captionFlow = `mdl 1;
create or modify microflow MyFirstModule.CaptionIf ($N: Integer) returns String
begin
  log info node 'X' 'start';
  if $N > 1 then
    return 'big';
  else
    return 'small';
  end if;
end;`

// captionedFlow is captionFlow with caption stated on the decision ("" for
// none).
func captionedFlow(caption, cond string) string {
	src := captionFlow
	if cond != "" {
		src = strings.Replace(src, "if $N > 1 then", "if "+cond+" then", 1)
	}
	if caption != "" {
		src = strings.Replace(src, "  if ", "  @caption '"+caption+"'\n  if ", 1)
	}
	return src
}

// captionSplit returns the stored decision of CaptionIf.
func captionSplit(t *testing.T, exec *Executor) *microflows.ExclusiveSplit {
	t.Helper()
	mf := loopEditFlowOf(t, exec, "CaptionIf")
	for _, obj := range mf.ObjectCollection.Objects {
		if s, ok := obj.(*microflows.ExclusiveSplit); ok {
			return s
		}
	}
	t.Fatal("CaptionIf has no decision")
	return nil
}

func TestCreateOrModify_DecisionCaptionIsSetInPlace(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, captionFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	before := flowObjectIDs(loopEditFlowOf(t, exec, "CaptionIf").ObjectCollection)
	splitID := captionSplit(t, exec).ID

	step := func(t *testing.T, src, wantCaption, wantExpr string) {
		t.Helper()
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		for _, v := range exec.CheckFlowVerdicts(prog) {
			t.Errorf("check -p predicts a refusal: %s", v.Message)
		}
		out.Reset()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("refused: %v", err)
		}
		if !strings.Contains(out.String(), "Modified microflow: MyFirstModule.CaptionIf (spliced") {
			t.Errorf("want the change spliced, got:\n%s", out.String())
		}
		split := captionSplit(t, exec)
		if split.ID != splitID {
			t.Errorf("the decision was rebuilt: $ID %s, was %s", split.ID, splitID)
		}
		if split.Caption != wantCaption {
			t.Errorf("caption = %q, want %q", split.Caption, wantCaption)
		}
		if c, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition); !ok || c.Expression != wantExpr {
			t.Errorf("condition = %#v, want %q", split.SplitCondition, wantExpr)
		}
		// Nothing else is rebuilt or moved.
		after := flowObjectIDs(loopEditFlowOf(t, exec, "CaptionIf").ObjectCollection)
		if len(after) != len(before) {
			t.Errorf("object count %d, was %d", len(after), len(before))
		}
		for id, p := range before {
			if q, ok := after[id]; !ok {
				t.Errorf("object %s is gone", id)
			} else if p != q {
				t.Errorf("object %s moved from %v to %v", id, p, q)
			}
		}
		// The twice-exec rule: the same statement again writes nothing.
		out.Reset()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("re-run: %v", err)
		}
		if !strings.Contains(out.String(), "Unchanged microflow: MyFirstModule.CaptionIf") {
			t.Errorf("re-run is not Unchanged:\n%s", out.String())
		}
	}

	t.Run("caption added", func(t *testing.T) {
		step(t, captionedFlow("Big?", ""), "Big?", "$N > 1")
	})
	t.Run("caption reworded", func(t *testing.T) {
		step(t, captionedFlow("Large?", ""), "Large?", "$N > 1")
	})
	t.Run("caption and condition changed together", func(t *testing.T) {
		step(t, captionedFlow("Huge?", "$N > 10"), "Huge?", "$N > 10")
	})
	t.Run("caption taken off reads as the condition", func(t *testing.T) {
		step(t, captionedFlow("", "$N > 10"), "$N > 10", "$N > 10")
	})

	// Control: a path-shape change is still refused, with the reason, and
	// writes nothing.
	t.Run("control: a branch's return taken away is still refused", func(t *testing.T) {
		split := captionSplit(t, exec)
		src := strings.Replace(captionedFlow("Big?", "$N > 10"), "    return 'big';\n", "    log info node 'X' 'big';\n", 1)
		src = strings.Replace(src, "  end if;\nend;", "  end if;\n  return 'big';\nend;", 1)
		err := agreeExec(t, exec, src)
		if err == nil || !strings.Contains(err.Error(), "cannot be spliced") {
			t.Fatalf("want a splice refusal, got %v", err)
		}
		if got := captionSplit(t, exec); got.ID != split.ID || got.Caption != split.Caption {
			t.Errorf("the refused statement wrote: %s %q", got.ID, got.Caption)
		}
	})
}
