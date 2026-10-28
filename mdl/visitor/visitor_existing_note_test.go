// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1695: `@annotation(existing: '…')` names a note already stored in
// the flow. It is its own key, never read as `text:`, and it attaches to the
// statement wherever it stands among the statement's annotations — a stored
// note is on the canvas already, so before `@position` it is not a free note.
func TestExistingNoteAnnotation(t *testing.T) {
	prog, errs := Build(`alter microflow M.F {
  insert after log node 'App' 'start' begin
    @annotation(existing: 'Validates the order')
    @position(400, 200)
    log info node 'App' 'middle';
  end;
};`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	alter := prog.Statements[0].(*ast.AlterFlowStmt)
	ann := ast.StatementAnnotations(alter.Operations[0].Body[0])
	if ann == nil {
		t.Fatal("no annotations")
	}
	if len(ann.FreeNotes) != 0 || len(ann.InvalidNotes) != 0 {
		t.Fatalf("free %#v, invalid %#v: want neither", ann.FreeNotes, ann.InvalidNotes)
	}
	if len(ann.Notes) != 1 {
		t.Fatalf("notes %#v: want one", ann.Notes)
	}
	if n := ann.Notes[0]; n.Existing != "Validates the order" || n.Text != "" || n.Label != "" {
		t.Errorf("note %#v: want Existing set and no text or label", n)
	}

	// Control: the same position for a note of its own makes it free.
	prog, errs = Build(`alter microflow M.F {
  insert after log node 'App' 'start' begin
    @annotation(text: 'Validates the order')
    @position(400, 200)
    log info node 'App' 'middle';
  end;
};`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	alter = prog.Statements[0].(*ast.AlterFlowStmt)
	if ann := ast.StatementAnnotations(alter.Operations[0].Body[0]); len(ann.FreeNotes) != 1 {
		t.Errorf("control: a note of its own before @position is free, got %#v", ann)
	}
}

// An empty text names nothing and is recorded, not dropped (MDL079).
func TestExistingNoteAnnotationEmptyIsInvalid(t *testing.T) {
	prog, errs := Build(`create microflow M.F ()
begin
  @annotation(existing: '')
  log info node 'App' 'x';
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	ann := ast.StatementAnnotations(body[0])
	if ann == nil || len(ann.InvalidNotes) == 0 || len(ann.Notes) != 0 {
		t.Errorf("want the empty `existing:` recorded as invalid, got %#v", ann)
	}
}
