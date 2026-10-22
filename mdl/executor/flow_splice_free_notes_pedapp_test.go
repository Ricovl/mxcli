// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Item 3 of saingge's comment on mendixlabs/mxcli#1517 (ako/mxcli#1399), last
// form: rewording a free note — one on the canvas, attached to nothing — was
// refused:
//
//	the free annotations change (note "free one" is now "free one, reworded");
//	the splice edits activities only.
//
// A free note is no part of any activity, so changing it touches nothing else:
// the stored free notes are replaced by the stated ones, and every activity,
// flow and attached note stays.

const freeNotesFlow = `mdl 1;
create or modify microflow MyFirstModule.FreeNotes ()
begin
  @annotation(text: 'free one', position: (100, 50), size: (150, 60))
  @position(300, 200)
  @annotation 'attached'
  log info node 'X' 'a';
  log info node 'X' 'b';
end;`

// freeNoteState returns the captions of FreeNotes' free notes (sorted), and
// the $IDs of everything else.
func freeNoteState(t *testing.T, exec *Executor) ([]string, map[model.ID]bool) {
	t.Helper()
	mf := loopEditFlowOf(t, exec, "FreeNotes")
	wired := map[model.ID]bool{}
	for _, af := range mf.ObjectCollection.AnnotationFlows {
		wired[af.OriginID], wired[af.DestinationID] = true, true
	}
	var free []string
	rest := map[model.ID]bool{}
	for _, obj := range mf.ObjectCollection.Objects {
		if n, ok := obj.(*microflows.Annotation); ok && !wired[n.ID] {
			free = append(free, n.Caption)
			continue
		}
		rest[obj.GetID()] = true
	}
	sort.Strings(free)
	return free, rest
}

func TestCreateOrModify_FreeNotesChangeInPlace(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, freeNotesFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	_, before := freeNoteState(t, exec)

	step := func(t *testing.T, src string, want ...string) {
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
		if !strings.Contains(out.String(), "Modified microflow: MyFirstModule.FreeNotes (set: free notes)") {
			t.Errorf("want the free notes reported as set:\n%s", out.String())
		}
		free, rest := freeNoteState(t, exec)
		if strings.Join(free, "|") != strings.Join(want, "|") {
			t.Errorf("free notes %v, want %v", free, want)
		}
		for id := range before {
			if !rest[id] {
				t.Errorf("object %s was rebuilt or removed", id)
			}
		}
		out.Reset()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("re-run: %v", err)
		}
		if !strings.Contains(out.String(), "Unchanged microflow: MyFirstModule.FreeNotes") {
			t.Errorf("re-run is not Unchanged:\n%s", out.String())
		}
	}

	t.Run("reworded", func(t *testing.T) {
		step(t, strings.Replace(freeNotesFlow, "'free one'", "'free one, reworded'", 1), "free one, reworded")
	})
	t.Run("added", func(t *testing.T) {
		src := strings.Replace(freeNotesFlow, "'free one'", "'free one, reworded'", 1)
		src = strings.Replace(src, "  @position(300, 200)\n",
			"  @annotation(text: 'free two', position: (500, 50), size: (150, 60))\n  @position(300, 200)\n", 1)
		step(t, src, "free one, reworded", "free two")
	})
	t.Run("taken away", func(t *testing.T) {
		src := strings.Replace(freeNotesFlow, "  @annotation(text: 'free one', position: (100, 50), size: (150, 60))\n", "", 1)
		step(t, src)
	})
	// The attached note is untouched throughout.
	if desc := describeFlow(t, exec, "FreeNotes"); !strings.Contains(desc, "@annotation 'attached'") {
		t.Errorf("the attached note is gone:\n%s", desc)
	}
}
