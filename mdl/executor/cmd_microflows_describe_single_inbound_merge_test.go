// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A merge with ONE incoming path is a node mxcli's own `exec` writes from
// ordinary MDL — `join finish; merge finish;` — and DESCRIBE walked straight
// through it, printing a warning that re-executing the description deletes it
// (mendixlabs/mxcli#923, ako/mxcli#1277). On the main line it is represented
// the way it was written, so the description rebuilds it and has nothing to
// warn about.
func TestDescribe_SingleInboundMergeOnTheMainLineIsRepresented(t *testing.T) {
	const src = `create microflow M.MF_RT_Chain () returns string as $Out
begin
  declare $Out string = 'a';
  if 1 = 1 then
    if 2 = 2 then set $Out = 'healthy'; join emit;
    else set $Out = 'watch'; join review; end if;
  else set $Out = 'low'; join review; end if;
  merge review; log info node 'T' 'review'; join emit;
  merge emit; log info node 'T' 'emit'; join finish;
  merge finish; return $Out;
end;`
	out, oc := describeBuilt(t, src)
	if strings.Contains(out, "WARNING") {
		t.Errorf("the description still warns:\n%s", out)
	}
	assertDescriptionRebuildsGraph(t, out, oc)
	if got, want := countMerges(rebuildDescription(t, out)), countMerges(oc); got != want {
		t.Errorf("the description rebuilds %d merges, the flow has %d:\n%s", got, want, out)
	}
}

// The simplest shape: one activity, a one-input merge, the end. Also the
// fixture of TestDroppedMergeWarnings_PassThroughMergeIsFlagged, built from MDL.
func TestDescribe_SingleInboundMergeAfterAnActivityIsRepresented(t *testing.T) {
	const src = `create microflow M.MF_Pass () returns Boolean
begin
  log info node 'T' 'one';
  join onward;
  merge onward;
  log info node 'T' 'two';
  return true;
end;`
	out, oc := describeBuilt(t, src)
	if strings.Contains(out, "WARNING") {
		t.Errorf("the description still warns:\n%s", out)
	}
	if got, want := countMerges(rebuildDescription(t, out)), countMerges(oc); got != want || want != 1 {
		t.Errorf("the description rebuilds %d merges, the flow has %d (want 1):\n%s", got, want, out)
	}
	// Executing the description again describes identically: a fixed point.
	again, _ := describeBuilt(t, "create microflow M.MF_Pass () returns Boolean\nbegin\n"+out+"\nend;")
	if again != out {
		t.Errorf("not a fixed point:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

func countMerges(oc *microflows.MicroflowObjectCollection) int {
	n := 0
	for _, o := range oc.Objects {
		switch v := o.(type) {
		case *microflows.ExclusiveMerge:
			n++
		case *microflows.LoopedActivity:
			if v.ObjectCollection != nil {
				n += countMerges(v.ObjectCollection)
			}
		}
	}
	return n
}

// rebuildDescription builds the flow a description body defines.
func rebuildDescription(t *testing.T, body string) *microflows.MicroflowObjectCollection {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.Described () returns string\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("description does not parse: %v\n%s", errs[0], body)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}, varTypes: map[string]string{}}
	return fb.buildFlowGraph(mf.Body, mf.ReturnType)
}
