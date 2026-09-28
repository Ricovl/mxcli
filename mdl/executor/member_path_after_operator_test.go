// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A member path after a multiplicative operator (`$a/X * $b/Y`) must be stored
// exactly as written. `/` sits at the same precedence as `*` in the MDL grammar,
// so the left-associative chain used to become `($a/X * $b) / Y`: the serializer
// then printed `$b / Y` as division, and a filter's iterator qualification read
// the stranded `Y` as a bare attribute and wrote `$b / $currentObject/Y`. The
// build and mx check stayed green; Mendix evaluates a different expression.
func TestMemberPathAfterMultiplicativeOperatorIsStoredAsWritten(t *testing.T) {
	const src = `create microflow MyFirstModule.Agg0 ($Feedbacks: List of FeedbackModule.Feedback, $One: FeedbackModule.Feedback)
returns Integer
begin
  $Area = sum($Feedbacks, $currentObject/ScreenWidth * $currentObject/ScreenHeight);
  $W = filter($Feedbacks, $currentObject/ScreenWidth * $currentObject/ScreenHeight > 3);
  declare $X Integer = $One/ScreenWidth * $One/ScreenHeight;
  declare $D Decimal = $One/ScreenWidth div $One/ScreenHeight * $One/Rating;
  declare $N Integer = 2 * -$One/ScreenWidth;
  return $Area;
end;`
	got := storedExpressions(t, src)

	for _, want := range []string{
		"sum: $currentObject/ScreenWidth * $currentObject/ScreenHeight",
		"filter: $currentObject/ScreenWidth * $currentObject/ScreenHeight > 3",
		"declare X: $One/ScreenWidth * $One/ScreenHeight",
		// div binds like *; the member path after it is navigation too.
		"declare D: $One/ScreenWidth div $One/ScreenHeight * $One/Rating",
		// A unary minus in front of the path keeps the path whole (already
		// correct before the fix; guards against a regression).
		"declare N: 2 * -$One/ScreenWidth",
	} {
		if !containsString(got, want) {
			t.Errorf("stored expression %q missing; stored: %q", want, got)
		}
	}
}

// Control: division by a parenthesised expression or a variable is not member
// navigation and must not be re-associated.
func TestSlashAfterNonPathIsNotMadeAMemberPath(t *testing.T) {
	const src = `create microflow MyFirstModule.Agg1 ($One: FeedbackModule.Feedback)
returns Integer
begin
  declare $P Decimal = $One/ScreenWidth * ($One/ScreenHeight) div 2;
  return 1;
end;`
	got := storedExpressions(t, src)
	want := "declare P: $One/ScreenWidth * ($One/ScreenHeight) div 2"
	if !containsString(got, want) {
		t.Errorf("stored expression %q missing; stored: %q", want, got)
	}
}

// storedExpressions builds each statement of a microflow through the flow
// builder and returns the expression strings the actions would store.
func storedExpressions(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected a CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes:     map[string]string{"Feedbacks": "List of FeedbackModule.Feedback", "One": "FeedbackModule.Feedback"},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{"Feedbacks": "List of FeedbackModule.Feedback"}},
	}
	for _, stmt := range mf.Body {
		fb.addStatement(stmt)
	}
	var got []string
	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch a := act.Action.(type) {
		case *microflows.AggregateListAction:
			got = append(got, "sum: "+a.Expression)
		case *microflows.ListOperationAction:
			if f, ok := a.Operation.(*microflows.FilterOperation); ok {
				got = append(got, "filter: "+f.Expression)
			}
		case *microflows.CreateVariableAction:
			got = append(got, "declare "+a.VariableName+": "+a.InitialValue)
		}
	}
	return got
}
