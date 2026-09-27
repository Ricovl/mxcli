// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// An elsif arm is lowered into a nested if in the else branch, and that nested
// if is a decision of its own on the canvas: it has a position, a caption and a
// merge. describe folds a lone-if else back into `elsif` (#750), so the arm has
// to be able to carry the same annotations the nested `if` carried, or the fold
// would lose the layout. They are written before the `elsif` keyword, the way
// every statement's annotations are written before the statement.
func TestElsifArmAnnotationsAttachToLoweredIf(t *testing.T) {
	prog, errs := Build(`create microflow M.ElsifAnn ($In: Integer) returns Integer
begin
  declare $N Integer = 0;
  @position(520, 200)
  @caption 'first'
  if $In = 1 then
    set $N = 10;
  @position(690, 400)
  @merge(1190, 400)
  @caption 'second'
  elsif $In = 2 then
    set $N = 20;
  @position(860, 550)
  elsif $In = 3 then
    set $N = 30;
  else
    set $N = 40;
  end if;
  return $N;
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	outer, ok := mf.Body[1].(*ast.IfStmt)
	if !ok {
		t.Fatalf("body[1] is %T, want *ast.IfStmt", mf.Body[1])
	}
	if outer.Annotations == nil || outer.Annotations.Caption != "first" {
		t.Fatalf("outer if lost its own annotations: %+v", outer.Annotations)
	}
	if len(outer.ThenBody) != 1 {
		t.Fatalf("outer then-body has %d statements, want 1 — the arm's annotations were read as a statement", len(outer.ThenBody))
	}

	second, ok := outer.ElseBody[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("outer else is %T, want the lowered elsif arm", outer.ElseBody[0])
	}
	a := second.Annotations
	if a == nil || a.Position == nil || a.Merge == nil {
		t.Fatalf("second arm annotations = %+v, want position, merge and caption", a)
	}
	if a.Position.X != 690 || a.Position.Y != 400 || a.Merge.X != 1190 || a.Merge.Y != 400 || a.Caption != "second" {
		t.Errorf("second arm annotations = pos %+v merge %+v caption %q", a.Position, a.Merge, a.Caption)
	}

	third, ok := second.ElseBody[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("second arm else is %T, want the third arm", second.ElseBody[0])
	}
	if third.Annotations == nil || third.Annotations.Position == nil || third.Annotations.Position.X != 860 {
		t.Errorf("third arm annotations = %+v, want position (860, 550)", third.Annotations)
	}
	if third.Annotations != nil && third.Annotations.Caption != "" {
		t.Errorf("third arm picked up another arm's caption %q", third.Annotations.Caption)
	}
}
