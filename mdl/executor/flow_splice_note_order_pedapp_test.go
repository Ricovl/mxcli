// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// Item 2 of saingge's comment on mendixlabs/mxcli#1517 (ako/mxcli#1399):
// with a stored note on the first `log`, restating the flow with
// `@annotation(text: 'hello note', …)` written before `@position(300, 200)`
// was refused with
//
//	the free annotations change (note "hello note" is not in the stored
//	flow); the splice edits activities only.
//
// An @annotation before a statement's @position is a free note — attached to
// nothing — which is where describe prints one, so the order is the syntax and
// cannot change. What was wrong is that the refusal did not say so: the note
// IS in the stored flow, attached to the statement it is written above. The
// refusal now names the ordering and how to keep the note attached.

const noteOrderFlow = `mdl 1;
create or modify microflow MyFirstModule.NoteOrder ()
begin
  @position(300, 200)
  @annotation(text: 'hello note', position: (200, 100), size: (150, 60))
  log info node 'X' 'a';
  log info node 'X' 'b';
end;`

func TestCreateOrModify_NoteBeforePositionNamesTheOrder(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, noteOrderFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	reordered := strings.Replace(noteOrderFlow,
		"  @position(300, 200)\n  @annotation(text: 'hello note', position: (200, 100), size: (150, 60))\n",
		"  @annotation(text: 'hello note', position: (200, 100), size: (150, 60))\n  @position(300, 200)\n", 1)
	const want = "write it after @position"
	checkSays(t, exec, reordered, want)
	err := agreeExec(t, exec, reordered)
	if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "before @position") {
		t.Fatalf("want the refusal to name the note's order, got %v", err)
	}

	// Control: a note that is new to the flow is a free note, and is written
	// as one (free notes change in place) — not refused as a misplaced
	// attached note.
	added := strings.Replace(noteOrderFlow, "  @position(300, 200)\n",
		"  @annotation(text: 'brand new', position: (100, 50), size: (150, 60))\n  @position(300, 200)\n", 1)
	if err := agreeExec(t, exec, added); err != nil {
		t.Fatalf("a new free note is refused: %v", err)
	}
	if desc := describeFlow(t, exec, "NoteOrder"); !strings.Contains(desc, "@annotation(text: 'brand new'") ||
		!strings.Contains(desc, "@annotation(text: 'hello note'") {
		t.Errorf("want the free note added and the attached one kept:\n%s", desc)
	}
}
