// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#905 (#897 item 4, rehearsal 3 class G2): a fragment whose custom
// error handler ends in its own `return` was refused by the splice — "an error
// handler in the fragment ends at an end event of its own … a return inside an
// error handler is not spliced yet" — because the handler is built by a child
// builder whose returns never reached the fragment's returnEndIDs. The
// handler's end event is a return the script states, so it is part of the
// fragment exactly like a guard clause's (#888): a new end event of the flow.
func TestCutFragment_HandlerReturnIsANewEndEvent(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LogStmt{
			Level:   ast.LogInfo,
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "risky"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustom,
				Body: []ast.MicroflowStatement{
					&ast.LogStmt{Level: ast.LogError, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "failed"}},
					&ast.ReturnStmt{},
				},
			},
		},
		&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "done"}},
	}
	fb := &flowBuilder{posX: 200, posY: 200, baseY: 200, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph(body, nil)
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Fatalf("build: %v", errs)
	}
	if fb.endsWithReturn {
		t.Fatal("a return inside an error handler does not end the fragment's main path")
	}
	frag, err := cutFragment(oc, fb.fallThroughEndID, fb.returnEndIDs)
	if err != nil {
		t.Fatalf("the handler's return was refused: %v", err)
	}
	var ends int
	for _, obj := range frag.Objects {
		if _, ok := obj.(*microflows.EndEvent); ok {
			ends++
		}
	}
	if ends != 1 {
		t.Errorf("%d end events in the fragment, want the handler's return alone", ends)
	}
	var errFlow bool
	for _, f := range frag.Flows {
		if f.IsErrorHandler {
			errFlow = true
		}
	}
	if !errFlow {
		t.Error("the fragment lost its error-handler flow")
	}
	if frag.Exit == "" {
		t.Error("the fragment does not lead on to the rest of the flow")
	}
}
