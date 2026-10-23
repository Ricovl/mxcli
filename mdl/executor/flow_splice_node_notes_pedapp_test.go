// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Item 3 of saingge's comment on mendixlabs/mxcli#1517 (ako/mxcli#1399):
// adding or rewording a note was refused when the note is on a decision —
//
//	the If at (360, 200) changes where its paths end or meet …
//
// — or on the final `return` —
//
//	the annotations on the Return at (1120, 200) change; the splice changes a
//	return's value only.
//
// A decision and an end event are changed in place, never replaced, so their
// notes are now changed in place too: the stored notes are taken off and the
// stated ones attached, and the node keeps its $ID and its flows.

const nodeNotesFlow = `mdl 1;
create or modify microflow MyFirstModule.NodeNotes ($L: String) returns String
begin
  if $L = empty then
    return 'e';
  end if;
  log info node 'X' 'plain';
  return 'done';
end;`

// nodeNotes returns the notes attached to the node of type T in NodeNotes,
// and that node's $ID, and the IDs of every other non-note object.
func nodeNotes[T microflows.MicroflowObject](t *testing.T, exec *Executor, match func(T) bool) ([]string, model.ID, map[model.ID]bool) {
	t.Helper()
	mf := loopEditFlowOf(t, exec, "NodeNotes")
	var target model.ID
	others := map[model.ID]bool{}
	captions := map[model.ID]string{}
	for _, obj := range mf.ObjectCollection.Objects {
		if n, ok := obj.(*microflows.Annotation); ok {
			captions[n.ID] = n.Caption
			continue
		}
		if o, ok := obj.(T); ok && match(o) {
			target = obj.GetID()
			continue
		}
		others[obj.GetID()] = true
	}
	var notes []string
	for _, af := range mf.ObjectCollection.AnnotationFlows {
		if af.DestinationID == target {
			notes = append(notes, captions[af.OriginID])
		} else if af.OriginID == target {
			notes = append(notes, captions[af.DestinationID])
		}
	}
	return notes, target, others
}

func TestCreateOrModify_NotesOnADecisionOrEndEvent(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, nodeNotesFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	isSplit := func(*microflows.ExclusiveSplit) bool { return true }
	isDone := func(e *microflows.EndEvent) bool { return e.ReturnValue == "'done'" }

	step := func(t *testing.T, src string) {
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
		if !strings.Contains(out.String(), "Modified microflow: MyFirstModule.NodeNotes (spliced") {
			t.Errorf("want a splice:\n%s", out.String())
		}
		out.Reset()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("re-run: %v", err)
		}
		if !strings.Contains(out.String(), "Unchanged microflow: MyFirstModule.NodeNotes") {
			t.Errorf("re-run is not Unchanged:\n%s", out.String())
		}
	}
	withSplitNote := func(note string) string {
		return strings.Replace(nodeNotesFlow, "  if $L", "  @annotation '"+note+"'\n  if $L", 1)
	}

	for _, tc := range []struct{ name, note string }{{"note added", "guard"}, {"note reworded", "guard clause"}} {
		t.Run("decision: "+tc.name, func(t *testing.T) {
			_, split, others := nodeNotes(t, exec, isSplit)
			step(t, withSplitNote(tc.note))
			notes, now, after := nodeNotes(t, exec, isSplit)
			if now != split {
				t.Errorf("the decision was replaced")
			}
			if len(notes) != 1 || notes[0] != tc.note {
				t.Errorf("decision notes %v, want [%s]", notes, tc.note)
			}
			// Drawn where a full build draws it, so describe needs no
			// geometry for it.
			if desc := describeFlow(t, exec, "NodeNotes"); !strings.Contains(desc, "@annotation '"+tc.note+"'\n") {
				t.Errorf("the note is not drawn at its default place:\n%s", desc)
			}
			for id := range others {
				if !after[id] {
					t.Errorf("object %s was rebuilt", id)
				}
			}
		})
	}
	t.Run("decision: note taken off", func(t *testing.T) {
		step(t, nodeNotesFlow)
		if notes, _, _ := nodeNotes(t, exec, isSplit); len(notes) != 0 {
			t.Errorf("decision notes %v, want none", notes)
		}
	})
	t.Run("end event: note added", func(t *testing.T) {
		_, end, _ := nodeNotes(t, exec, isDone)
		src := strings.Replace(nodeNotesFlow, "  return 'done';", "  @annotation 'the end'\n  return 'done';", 1)
		step(t, src)
		notes, now, _ := nodeNotes(t, exec, isDone)
		if now != end || len(notes) != 1 || notes[0] != "the end" {
			t.Errorf("end event %s (was %s) notes %v, want it kept with [the end]", now, end, notes)
		}
	})

	// Control: a note shared with another activity (an id) is still refused —
	// changing it here would change it there too.
	t.Run("control: a stored shared note is not changed", func(t *testing.T) {
		shared := strings.Replace(nodeNotesFlow, "  if $L", "  @annotation(id: s1, text: 'shared')\n  if $L", 1)
		shared = strings.Replace(shared, "  log info node 'X' 'plain';", "  @annotation(id: s1)\n  log info node 'X' 'plain';", 1)
		if err := agreeExec(t, exec, "mdl 1;\ndrop microflow MyFirstModule.NodeNotes;\n"+strings.TrimPrefix(shared, "mdl 1;\n")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		reworded := strings.Replace(shared, "text: 'shared'", "text: 'shared, reworded'", 1)
		if err := agreeExec(t, exec, reworded); err == nil {
			t.Fatal("want a refusal for a reworded shared note")
		}
	})
}
