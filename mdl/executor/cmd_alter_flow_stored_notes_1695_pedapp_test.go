// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1695 (mendixlabs/mxcli#1517, comment item 3): a fragment of
// `alter microflow … insert` could not attach its new statement to a note
// already stored in the flow. `@annotation(id: n1)` was refused — "refers to
// an annotation that has not been declared" — because the fragment is built
// alone, and the labels describe prints are re-derived on every describe, so a
// saved one could later name a different note. A stored note is now named by
// its text: `@annotation(existing: '…')`, which must match exactly one note.

const storedNoteFlow = `mdl 1;
create microflow MyFirstModule.NoteAttach ()
begin
  @annotation 'Validates the order'
  log info node 'App' 'start';
  @annotation(text: 'Below the row', position: (560, 330))
  log info node 'App' 'done';
end;`

const storedNoteTwinsFlow = `mdl 1;
create microflow MyFirstModule.NoteTwins ()
begin
  @annotation 'Check'
  log info node 'App' 'a';
  @annotation 'Check'
  log info node 'App' 'b';
end;`

func openStoredNoteFixture(t *testing.T) (*Executor, *bytes.Buffer, string) {
	t.Helper()
	exec, out, dir := openPedAppCopy(t)
	for _, src := range []string{storedNoteFlow, storedNoteTwinsFlow} {
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
	}
	return exec, out, dir
}

// notesReading returns the flow's notes whose text is text.
func notesReading(mf *microflows.Microflow, text string) []*microflows.Annotation {
	var out []*microflows.Annotation
	for _, obj := range mf.ObjectCollection.Objects {
		if n, ok := obj.(*microflows.Annotation); ok && n.Caption == text {
			out = append(out, n)
		}
	}
	return out
}

// linesFrom returns the note lines leaving note id.
func linesFrom(mf *microflows.Microflow, id model.ID) []*microflows.AnnotationFlow {
	var out []*microflows.AnnotationFlow
	for _, af := range mf.ObjectCollection.AnnotationFlows {
		if af.OriginID == id {
			out = append(out, af)
		}
	}
	return out
}

// logActivity returns the ID of the log activity whose message is text.
func logActivity(t *testing.T, mf *microflows.Microflow, text string) model.ID {
	t.Helper()
	for _, obj := range mf.ObjectCollection.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		lg, ok := act.Action.(*microflows.LogMessageAction)
		if !ok || lg.MessageTemplate == nil {
			continue
		}
		for _, s := range lg.MessageTemplate.Translations {
			if s == text {
				return act.ID
			}
		}
	}
	t.Fatalf("no log activity says %q", text)
	return ""
}

// flowUnitBytes returns MyFirstModule.name's stored bytes.
func flowUnitBytes(t *testing.T, exec *Executor, name string) []byte {
	t.Helper()
	mf := loopEditFlowOf(t, exec, name)
	raw, err := exec.newExecContext(context.Background()).Backend.GetRawUnitBytes(mf.ID)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), raw...)
}

// staticErrors is what `mxcli check` reports as an error without a project.
func staticErrors(t *testing.T, src string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	return strings.Join(out, "\n")
}

const attachByText = `mdl 1;
alter microflow MyFirstModule.NoteAttach {
  insert after log node 'App' 'start' begin
    @annotation(existing: 'Validates the order')
    log info node 'App' 'middle';
  end;
};`

func TestAlterMicroflow_AttachesAStoredNoteByItsText(t *testing.T) {
	exec, out, _ := openStoredNoteFixture(t)
	before := loopEditFlowOf(t, exec, "NoteAttach")
	stored := notesReading(before, "Validates the order")
	if len(stored) != 1 {
		t.Fatalf("setup: want one stored note, got %d", len(stored))
	}
	note := stored[0]
	if s := staticErrors(t, attachByText); s != "" {
		t.Errorf("check must accept the statement, reported:\n%s", s)
	}
	predictedAgree(t, exec, attachByText, "")

	after := loopEditFlowOf(t, exec, "NoteAttach")
	notes := notesReading(after, "Validates the order")
	if len(notes) != 1 {
		t.Fatalf("the note must be attached, not copied: %d notes read its text", len(notes))
	}
	if got := notes[0]; got.ID != note.ID || got.Position != note.Position || got.Size != note.Size {
		t.Errorf("the stored note must keep its $ID, place and size: was %s at %v %v, is %s at %v %v",
			note.ID, note.Position, note.Size, got.ID, got.Position, got.Size)
	}
	middle := logActivity(t, after, "middle")
	start := logActivity(t, after, "start")
	dests := map[model.ID]bool{}
	for _, af := range linesFrom(after, note.ID) {
		dests[af.DestinationID] = true
	}
	if len(dests) != 2 || !dests[middle] || !dests[start] {
		t.Errorf("the note must be attached to the stored activity and the inserted one, its lines go to %v "+
			"(start %s, middle %s)", dests, start, middle)
	}

	// describe prints the note as one shared note, and its statement re-runs
	// to nothing: the flow is what a whole-flow statement would write.
	desc := describeFlow(t, exec, "NoteAttach")
	if !strings.Contains(desc, "@annotation(id: n1, text: 'Validates the order'") ||
		!strings.Contains(desc, "@annotation(id: n1)\n  log node 'App' 'middle';") {
		t.Errorf("describe must print one note shared by both activities:\n%s", desc)
	}
	out.Reset()
	if err := agreeExec(t, exec, desc); err != nil {
		t.Fatalf("describe → exec: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged microflow: MyFirstModule.NoteAttach") {
		t.Errorf("describe → exec must leave the flow unchanged, got:\n%s", out.String())
	}
}

// Control: the same insert stating the note as text makes a second note,
// which is what the one-note-same-$ID assertion above tells apart.
func TestAlterMicroflow_StatedNoteTextIsANewNote(t *testing.T) {
	exec, _, _ := openStoredNoteFixture(t)
	predictedAgree(t, exec, strings.Replace(attachByText,
		"@annotation(existing: 'Validates the order')", "@annotation 'Validates the order'", 1), "")
	if n := len(notesReading(loopEditFlowOf(t, exec, "NoteAttach"), "Validates the order")); n != 2 {
		t.Errorf("control: `@annotation 'text'` creates a note of its own, want 2 notes reading the text, got %d", n)
	}
}

// The line is drawn where the two are once the fragment is in place, from the
// sides that face each other: a note below the row is left by its top edge.
func TestAlterMicroflow_StoredNoteLineFacesTheNote(t *testing.T) {
	exec, _, _ := openStoredNoteFixture(t)
	predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.NoteAttach {
  insert after log node 'App' 'done' begin
    @annotation(existing: 'Below the row')
    log info node 'App' 'later';
  end;
};`, "")
	mf := loopEditFlowOf(t, exec, "NoteAttach")
	notes := notesReading(mf, "Below the row")
	if len(notes) != 1 {
		t.Fatalf("want one note reading 'Below the row', got %d", len(notes))
	}
	later := logActivity(t, mf, "later")
	for _, af := range linesFrom(mf, notes[0].ID) {
		if af.DestinationID != later {
			continue
		}
		if s := af.SidesOrDefault(); s.Origin != AnchorTop || s.Destination != AnchorBottom {
			t.Errorf("a note below the row is joined top to bottom, got origin %d, destination %d", s.Origin, s.Destination)
		}
		return
	}
	t.Errorf("no line from the note to the inserted activity")
}

// A replacement attached to the note its target already carries gets that
// note's one line, not a second one.
func TestAlterMicroflow_ReplaceAttachedToTheReplacedNote(t *testing.T) {
	exec, _, _ := openStoredNoteFixture(t)
	note := notesReading(loopEditFlowOf(t, exec, "NoteAttach"), "Validates the order")[0]
	predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.NoteAttach {
  replace log node 'App' 'start' with begin
    @annotation(existing: 'Validates the order')
    log info node 'App' 'started';
  end;
};`, "")
	mf := loopEditFlowOf(t, exec, "NoteAttach")
	lines := linesFrom(mf, note.ID)
	if len(lines) != 1 || lines[0].DestinationID != logActivity(t, mf, "started") {
		t.Errorf("want the note's one line on the replacement, got %d lines", len(lines))
	}
}

// A text that names no stored note, or several, is refused by check and exec
// alike, and nothing is written.
func TestAlterMicroflow_StoredNoteTextMustNameOneNote(t *testing.T) {
	exec, _, _ := openStoredNoteFixture(t)
	t.Run("no note reads the text", func(t *testing.T) {
		raw := flowUnitBytes(t, exec, "NoteAttach")
		src := strings.Replace(attachByText, "existing: 'Validates the order'", "existing: 'Validates the orders'", 1)
		predictedAgree(t, exec, src, "names no note of the stored flow")
		scopeAgree(t, exec, src, `The flow's notes: "Validates the order" at (`)
		if !bytes.Equal(raw, flowUnitBytes(t, exec, "NoteAttach")) {
			t.Errorf("a refused statement wrote the flow")
		}
	})
	t.Run("two notes read the text", func(t *testing.T) {
		raw := flowUnitBytes(t, exec, "NoteTwins")
		src := `mdl 1;
alter microflow MyFirstModule.NoteTwins {
  insert after log node 'App' 'a' begin
    @annotation(existing: 'Check')
    log info node 'App' 'between';
  end;
};`
		predictedAgree(t, exec, src, "names 2 notes of the stored flow")
		scopeAgree(t, exec, src, "attached to `log node 'App' 'a'`")
		scopeAgree(t, exec, src, "attached to `log node 'App' 'b'`")
		if !bytes.Equal(raw, flowUnitBytes(t, exec, "NoteTwins")) {
			t.Errorf("a refused statement wrote the flow")
		}
	})
}

// `existing:` stands alone, and only a fragment has a stored flow: check (no
// project needed) and exec refuse the rest with the same words.
func TestAlterMicroflow_ExistingNoteMisuseRefused(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"beside text", strings.Replace(attachByText, "existing: 'Validates the order'",
			"existing: 'Validates the order', text: 'other'", 1), "so it takes no `text:` beside it"},
		{"beside position", strings.Replace(attachByText, "existing: 'Validates the order'",
			"existing: 'Validates the order', position: (10, 10)", 1), "so it takes no `position:` beside it"},
		{"in a whole flow", `mdl 1;
create microflow MyFirstModule.NoteWhole ()
begin
  @annotation(existing: 'Validates the order')
  log info node 'App' 'x';
end;`, "which only a fragment of `alter microflow|nanoflow … insert|replace` has"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec, _, _ := openStoredNoteFixture(t)
			if s := staticErrors(t, c.src); !strings.Contains(s, "MDL079") || !strings.Contains(s, c.want) {
				t.Errorf("check must refuse with %q, reported:\n%s", c.want, s)
			}
			if err := agreeExec(t, exec, c.src); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("exec must refuse with %q, got: %v", c.want, err)
			}
		})
	}
}

// Regression: a fragment's own notes — by text, and shared through `id:` —
// mean what they meant, and an `id:` the fragment does not declare is now
// refused by check as well as exec, pointing at `existing:`.
func TestAlterMicroflow_FragmentNoteLabelsUnchanged(t *testing.T) {
	t.Run("a note the fragment declares and shares", func(t *testing.T) {
		exec, _, _ := openStoredNoteFixture(t)
		predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.NoteAttach {
  insert after log node 'App' 'start' begin
    @annotation(id: n1, text: 'shared by the two')
    log info node 'App' 'one';
    @annotation(id: n1)
    log info node 'App' 'two';
    @annotation 'only the third'
    log info node 'App' 'three';
  end;
};`, "")
		mf := loopEditFlowOf(t, exec, "NoteAttach")
		shared := notesReading(mf, "shared by the two")
		if len(shared) != 1 || len(linesFrom(mf, shared[0].ID)) != 2 {
			t.Errorf("want one new note with two lines, got %d notes", len(shared))
		}
		if own := notesReading(mf, "only the third"); len(own) != 1 || len(linesFrom(mf, own[0].ID)) != 1 {
			t.Errorf("want one new note on the third activity, got %d", len(own))
		}
	})
	t.Run("an id the fragment does not declare", func(t *testing.T) {
		exec, _, _ := openStoredNoteFixture(t)
		src := strings.Replace(attachByText, "@annotation(existing: 'Validates the order')", "@annotation(id: n1)", 1)
		if s := staticErrors(t, src); !strings.Contains(s, "`@annotation(id: n1)` refers to a note that has not been declared above it") {
			t.Errorf("check must refuse an undeclared id in a fragment, reported:\n%s", s)
		}
		err := agreeExec(t, exec, src)
		if err == nil || !strings.Contains(err.Error(), "has not been declared") ||
			!strings.Contains(err.Error(), "@annotation(existing: '…')") {
			t.Errorf("exec must refuse it and point at `existing:`, got: %v", err)
		}
	})
}

// A note's line breaks are CRLF as Studio Pro stores them; a script saved with
// LF names the same note (mendixlabs/mxcli#1505). Any other difference is no
// match.
func TestStoredNoteNamedMatchesLineBreaksEitherWay(t *testing.T) {
	note := func(id, text string) *microflows.Annotation {
		return &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)}}, Caption: text}
	}
	a := &alterFlowContext{mf: &microflows.Microflow{ObjectCollection: &microflows.MicroflowObjectCollection{}}}
	notes := []*microflows.Annotation{note("n1", "first line\r\nsecond line"), note("n2", "other")}
	got, err := a.storedNoteNamed(notes, "first line\nsecond line")
	if err != nil || got.ID != "n1" {
		t.Errorf("an LF text must name the CRLF note, got %v, %v", got, err)
	}
	if _, err := a.storedNoteNamed(notes, "first line second line"); err == nil {
		t.Error("control: a text that differs in more than its line breaks names no note")
	}
}
