// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// An activity with a custom error handler (`on error [without rollback] begin
// … end error`) is drawn as the activity, an error flow out of it, the
// handler's activities, and — unless the handler returns — a merge where the
// handler's path rejoins the flow. The splice used to refuse any replace of
// such an activity, since the handler would be left with no activity, and
// `create or modify` treated the handler's body as part of the activity, so a
// change of either was refused and each mode advised the other
// (mendixlabs/mxcli#1489).
//
// Both are now spliced. The activity replaced by one stating the same handler
// is swapped in place, and the stored handler stays (mfmutator,
// replaceKeepingHandler); the handler's body is diffed like a branch of an
// `if`, its activities being top-level nodes of the stored graph. A change of
// the handler's kind is still refused.

// errorHandling is a statement's `on error` clause, nil when it has none.
func errorHandling(st ast.MicroflowStatement) *ast.ErrorHandlingClause {
	v := reflectElem(st)
	if !v.IsValid() {
		return nil
	}
	f := v.FieldByName("ErrorHandling")
	if !f.IsValid() {
		return nil
	}
	eh, _ := f.Interface().(*ast.ErrorHandlingClause)
	return eh
}

// withHandlerBody is a copy of st whose `on error` clause holds body.
func withHandlerBody(st ast.MicroflowStatement, body []ast.MicroflowStatement) ast.MicroflowStatement {
	v := reflect.ValueOf(st)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return st
	}
	c := reflect.New(v.Elem().Type())
	c.Elem().Set(v.Elem())
	f := c.Elem().FieldByName("ErrorHandling")
	eh, _ := f.Interface().(*ast.ErrorHandlingClause)
	if eh == nil {
		return st
	}
	clause := *eh
	clause.Body = body
	f.Set(reflect.ValueOf(&clause))
	out, _ := c.Interface().(ast.MicroflowStatement)
	return out
}

// sameHandlerShell reports whether two statements are the same kind of
// activity with a custom error handler of the same kind, differing in the
// activity, inside the handler, or both.
func sameHandlerShell(declared, stored ast.MicroflowStatement) bool {
	if statementKind(declared) != statementKind(stored) || !customErrorHandler(declared) || !customErrorHandler(stored) {
		return false
	}
	return errorHandling(declared).Type == errorHandling(stored).Type
}

// handlerEdit diffs an activity with a custom error handler against the
// stored one (sameHandlerShell): a changed activity is replaced in place,
// keeping the stored handler, and the handler's body is diffed as a
// statement list of its own.
func (pd *patchDiff) handlerEdit(declared, stored ast.MicroflowStatement) error {
	sBody := errorHandling(stored).Body
	shell := withHandlerBody(declared, sBody)
	if !declaredMatches(shell, stored) {
		if sameExceptPositions(shell, stored) {
			if err := pd.moved(shell, stored); err != nil {
				return err
			}
		} else {
			c, err := pd.loc.locate(stored)
			if err != nil {
				return err
			}
			body, replaceNotes, err := keepStoredNotes(stored, []ast.MicroflowStatement{shell})
			if err != nil {
				return err
			}
			if replaceNotes {
				return cannotSplice("the notes on the %s change with the activity; the splice keeps an error handler "+
					"only on an activity replaced by itself, so change the notes and the activity in two runs", describeAt(stored))
			}
			pd.add(ast.AlterFlowReplace, c, body)
			pd.ops[len(pd.ops)-1].KeepHandler = true
		}
	}
	return pd.statements(errorHandling(declared).Body, sBody)
}

// keepStoredHandler returns the fragment a replace of target is spliced with
// when target has a custom error handler and the replacement is one statement
// stating the same handler: the replacement's activity alone, which the
// mutator swaps in place, keeping the stored handler. Any other fragment is
// returned as built — the mutator refuses it, since the stored handler would
// be left with no activity — except one stating another handler, refused here
// with what works instead.
func (a *alterFlowContext) keepStoredHandler(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate,
	frag *backend.MicroflowFragment) (*backend.MicroflowFragment, error) {
	if len(op.Body) != 1 || !customErrorHandler(op.Body[0]) ||
		!microflows.IsCustomErrorHandling(microflows.ObjectErrorHandlingType(target.Object)) {
		return frag, nil
	}
	if !op.KeepHandler {
		stored, err := a.storedStatementAt(ctx, target)
		if err != nil {
			return nil, err
		}
		if !customErrorHandler(stored) || !sameIgnoringLayout(errorHandling(op.Body[0]), errorHandling(stored)) {
			return nil, fmt.Errorf("the replacement states another error handler than the stored one; the splice "+
				"keeps the stored handler, so state it as `describe` prints it, and change the handler's activities "+
				"with operations of their own, by the handles `describe %s %s with handles` prints",
				a.stmt.Kind(), a.stmt.Name)
		}
	}
	for _, obj := range frag.Objects {
		if obj.GetID() == frag.Entry {
			return &backend.MicroflowFragment{Objects: []microflows.MicroflowObject{obj}, Entry: frag.Entry, Exit: frag.Entry}, nil
		}
	}
	return nil, fmt.Errorf("the replacement's activity is not in its fragment")
}

// storedStatementAt returns the stored statement drawn as target: the one
// whose @position, in the flow's full-layout description, is where target is
// drawn. A statement inside a loop body is not looked for; its coordinates are
// relative to the loop.
func (a *alterFlowContext) storedStatementAt(ctx *ExecContext, target mfmutator.Candidate) (ast.MicroflowStatement, error) {
	if a.storedDesc == nil {
		st, err := describedFlowStmt(ctx, &flowDecl{nanoflow: a.stmt.Nanoflow, name: a.stmt.Name}, a)
		if err != nil {
			return nil, fmt.Errorf("the stored flow's description cannot be read: %v", err)
		}
		a.storedDesc = st
	}
	p := target.Object.GetPosition()
	if found := statementDrawnAt(storedBody(a.storedDesc), p); found != nil {
		return found, nil
	}
	return nil, fmt.Errorf("no stored statement is drawn at (%d, %d), where %s is", p.X, p.Y, target.Statement)
}

// statementDrawnAt finds the statement whose @position is p, in stmts, the
// branches of an `if` and the bodies of error handlers.
func statementDrawnAt(stmts []ast.MicroflowStatement, p model.Point) ast.MicroflowStatement {
	for _, st := range stmts {
		if ann := statementAnnotations(st); ann != nil && ann.Position != nil &&
			ann.Position.X == p.X && ann.Position.Y == p.Y {
			return st
		}
		var inner [][]ast.MicroflowStatement
		if ifs, ok := st.(*ast.IfStmt); ok {
			inner = append(inner, ifs.ThenBody, ifs.ElseBody)
		}
		if eh := errorHandling(st); eh != nil {
			inner = append(inner, eh.Body)
		}
		for _, body := range inner {
			if found := statementDrawnAt(body, p); found != nil {
				return found
			}
		}
	}
	return nil
}
