// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#767. The flow LEAVING an if's closing merge has an anchor of its
// own: when the next decision starts a new row it leaves the merge's bottom and
// enters the decision's top. The full description wrote the entering side
// (`@anchor(to: top)` on the next if) but had nowhere to write the leaving one,
// so re-executing it drew the flow out of the merge's right side and back
// across the row above. Every reader of the full description inherited that:
// `layout flows`, the ELK view, diff, `with handles`, and create or modify's
// diff.
//
// The complex-layout example is the reproduction the issue names: its long
// validation chains wrap onto new rows, and each wrap is a merge-to-split flow.
func TestDescribe_MergeExitAnchorRoundTrips(t *testing.T) {
	exec, out := describeExecutor(t)
	src, err := os.ReadFile("../../mdl-examples/doctype-tests/02c-complex-layout-examples.mdl")
	if err != nil {
		t.Fatal(err)
	}
	run(t, exec, string(src))

	for _, name := range []string{
		"CxLayout.CX_VAL_Factory",
		"CxLayout.CX_VAL_EmailTemplate",
		// Controls: flows of the same file with no merge-to-split wrap, which
		// round-tripped before the fix as well.
		"CxLayout.CX_POST_SurveySubmission",
		"CxLayout.CX_ACT_BatchProcessFactories",
	} {
		t.Run(name, func(t *testing.T) {
			stored := fullGeometry(t, exec, "microflow", name)
			asCopy := func(mdl, suffix string) []string {
				copyName := name + suffix
				script := strings.Replace(withoutGrant(mdl), "microflow "+name+" ", "microflow "+copyName+" ", 1)
				if !strings.Contains(script, copyName) {
					t.Fatalf("could not rename the described microflow:\n%s", script)
				}
				run(t, exec, script)
				return fullGeometry(t, exec, "microflow", copyName)
			}

			full, _, err := describeMicroflowToString(exec.newExecContext(t.Context()), flowQN(name))
			if err != nil {
				t.Fatal(err)
			}
			geometryDiff(t, "re-executing the full description", stored, asCopy(full, "_Full"))

			canonical := describeText(t, exec, out, "describe microflow "+name+";")
			geometryDiff(t, "re-executing the canonical description", stored, asCopy(canonical, "_Canonical"))
		})
	}
}

// geometryDiff is sameGeometry reporting only the lines that differ: these
// flows have close to a hundred objects and flows each.
func geometryDiff(t *testing.T, what string, before, after []string) {
	t.Helper()
	inBefore, inAfter := map[string]bool{}, map[string]bool{}
	for _, l := range before {
		inBefore[l] = true
	}
	for _, l := range after {
		inAfter[l] = true
	}
	var diff []string
	for _, l := range before {
		if !inAfter[l] {
			diff = append(diff, "- "+l)
		}
	}
	for _, l := range after {
		if !inBefore[l] {
			diff = append(diff, "+ "+l)
		}
	}
	if len(diff) > 0 || len(before) != len(after) {
		t.Errorf("%s moved the flow (- stored, + rebuilt):\n  %s", what, strings.Join(diff, "\n  "))
	}
}

// mergeExitSides returns the origin side of every flow leaving an
// ExclusiveMerge of the stored flow, sorted.
func mergeExitSides(t *testing.T, exec *Executor, name string) []string {
	t.Helper()
	_, g := rawFlow(t, exec, "microflow", name)
	idx := indexFlowGraph(g.objects)
	var out []string
	for _, f := range idx.flow {
		if _, ok := idx.object[f.OriginID].(*microflows.ExclusiveMerge); ok {
			out = append(out, anchorSideKeyword(f.OriginConnectionIndex))
		}
	}
	sort.Strings(out)
	return out
}

// `@anchor(from: X)` on an if is the flow leaving the statement. With a
// closing merge that is the flow out of the merge. Inside a branch body the
// builder already honoured it; at the top level it was dropped (#767). The
// unannotated if is the control.
func TestBuild_IfFromAnchorIsTheMergeExit(t *testing.T) {
	exec, _ := describeExecutor(t)
	for _, tc := range []struct {
		name, anchor string
		want         []string
	}{
		{"MyFirstModule.MergeExitDefault", "", []string{"right"}},
		{"MyFirstModule.MergeExitBottom", "@anchor(from: bottom)\n  ", []string{"bottom"}},
		{"MyFirstModule.MergeExitBottomWithTo", "@anchor(from: bottom, to: top)\n  ", []string{"bottom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run(t, exec, `create microflow `+tc.name+` ($Flag: boolean)
begin
  `+tc.anchor+`if $Flag then
    log info node 'Test' 'yes';
  else
    log info node 'Test' 'no';
  end if;
  log info node 'Test' 'after';
end;`)
			if got := mergeExitSides(t, exec, tc.name); strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("merge exit sides = %v, want %v", got, tc.want)
			}
		})
	}
}

// The exit anchor belongs to the if that set it. Inside a case branch the
// branch's own anchor bookkeeping carries it, so it must not also survive the
// case and land on the flow out of the CASE's merge (or anything after it).
func TestBuild_IfExitAnchorDoesNotLeakOutOfCase(t *testing.T) {
	exec, _ := describeExecutor(t)
	const name = "MyFirstModule.MergeExitInCase"
	run(t, exec, `create enumeration MyFirstModule.MergeExitKind (One 'One', Two 'Two');`)
	run(t, exec, `create microflow `+name+` ($Flag: boolean, $Kind: enum MyFirstModule.MergeExitKind)
begin
  if $Flag then
    case $Kind
      when One then
        @anchor(from: bottom)
        if $Flag then
          log info node 'Test' 'one-yes';
        else
          log info node 'Test' 'one-no';
        end if;
        log info node 'Test' 'one-after';
      when Two then
        log info node 'Test' 'two';
    end case;
    log info node 'Test' 'after-case';
  end if;
  log info node 'Test' 'after';
end;`)
	got := mergeExitSides(t, exec, name)
	bottoms := 0
	for _, side := range got {
		if side == "bottom" {
			bottoms++
		}
	}
	if bottoms != 1 {
		t.Errorf("want exactly the annotated if's merge to leave from the bottom, got merge exit sides %v", got)
	}
}
