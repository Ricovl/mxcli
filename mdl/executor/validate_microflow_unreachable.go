// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkUnreachableStatements (MDL-FLOW05) refuses a statement that can never run
// because the statement before it in the same block ends the path: `return`,
// `raise error`, `break`, `continue`, `join`, an `if` whose two branches both
// end, or a `while true` with no `break` of its own (mendixlabs/mxcli#1491).
//
// The builder writes such a statement anyway, wired out of an End event, a
// Break/Continue/Error event or a merge-bound path, and the build refuses the
// result. Measured on mxbuild 11.13.0, one microflow per shape, the error code
// depending only on what the dead statement hangs off:
//
//	log after `return` in an if           CE0709
//	log after `break` / `continue`        CE0709
//	log after `raise error` / `return` in an error handler   CE0105 + CE0709
//	log after `join`                      CE0105 + CE0709 x3
//	log after an if/else that returns on both sides   CE0104 + CE0105
//	log after a `while true` left only by return      CE0104 + CE0105
//	`return` after that same loop         CE0104 "End event is unreachable."
//
// and the controls, all 0 errors: the same loop with nothing after it, a
// `while true` with a `break` and a `return` after it, an if WITHOUT else that
// returns followed by code, and a `merge` after a `return` that a `join`
// reaches. That last one is why a `merge` restarts reachability: it is the one
// statement a path can arrive at from elsewhere.
//
// Also measured: a `while true` whose only break is in a NESTED loop is the
// same CE0104 on the `return` after it, and so is a nanoflow (CE0709).
//
// Splits are deliberately not treated as ending the path even when every case
// does. An enumeration split whose cases all return does make the next
// statement CE0104 — but mxbuild reports CE0079 for the missing (empty) outcome
// with it, and the fix is that outcome, not deleting the statement; telling the
// author the code is dead would send them the wrong way. Missing a dead
// statement costs a later build error; claiming one that is live would refuse a
// script that builds, which is the expensive direction.
//
// Only the first dead statement of a block is reported — the rest of the block
// is dead for the same reason, and one message names the fix.
func (v *microflowValidator) checkUnreachableStatements(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() {
		return
	}
	v.walkUnreachable(body)
}

// walkUnreachable reports the first dead statement of stmts and of every block
// nested in it, and returns whether stmts as a whole ends the path.
func (v *microflowValidator) walkUnreachable(stmts []ast.MicroflowStatement) bool {
	ended := false
	var after ast.MicroflowStatement
	reported := false
	for _, s := range stmts {
		if _, ok := s.(*ast.MergeStmt); ok {
			// A join elsewhere can arrive here, so the path is live again.
			ended, reported = false, false
			continue
		}
		if ended && !reported {
			reported = true
			v.addViolation("MDL-FLOW05", linter.SeverityError,
				"this "+statementKindWord(s)+" can never run: in the same block it follows "+
					endsPathDescription(after)+". mxbuild refuses the flow (CE0104 unreachable / CE0105 / CE0709)",
				unreachableSuggestion(after))
		}
		if v.statementEndsPath(s) {
			ended, after = true, s
		}
	}
	return ended
}

// statementEndsPath reports whether no path continues past s to the statement
// after it, walking the blocks inside s for their own dead statements on the way.
func (v *microflowValidator) statementEndsPath(s ast.MicroflowStatement) bool {
	if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
		v.walkUnreachable(eh.Body)
	}
	switch st := s.(type) {
	case *ast.ReturnStmt, *ast.RaiseErrorStmt, *ast.BreakStmt, *ast.ContinueStmt, *ast.JoinStmt:
		return true
	case *ast.IfStmt:
		thenEnds := v.walkUnreachable(st.ThenBody)
		elseEnds := v.walkUnreachable(st.ElseBody)
		return len(st.ElseBody) > 0 && thenEnds && elseEnds
	case *ast.WhileStmt:
		v.walkUnreachable(st.Body)
		return isUnconditionalTrueWhile(st) && !breaksOutOfLoop(st.Body)
	case *ast.LoopStmt:
		v.walkUnreachable(st.Body)
	case *ast.EnumSplitStmt:
		for _, c := range st.Cases {
			v.walkUnreachable(c.Body)
		}
		v.walkUnreachable(st.ElseBody)
	case *ast.InheritanceSplitStmt:
		for _, c := range st.Cases {
			v.walkUnreachable(c.Body)
		}
		v.walkUnreachable(st.ElseBody)
	}
	return false
}

// breaksOutOfLoop reports whether stmts holds a `break` that leaves the loop
// whose body it is. It looks into every nested block except another loop's body,
// where a break leaves that loop instead. It is deliberately wider than
// containsBreakForCurrentLoop, which does not look into splits or handlers: a
// break found here only stops a report, so seeing too many is the safe side.
func breaksOutOfLoop(stmts []ast.MicroflowStatement) bool {
	for _, s := range stmts {
		if eh := stmtErrorHandling(s); eh != nil && breaksOutOfLoop(eh.Body) {
			return true
		}
		switch st := s.(type) {
		case *ast.BreakStmt:
			return true
		case *ast.IfStmt:
			if breaksOutOfLoop(st.ThenBody) || breaksOutOfLoop(st.ElseBody) {
				return true
			}
		case *ast.EnumSplitStmt:
			for _, c := range st.Cases {
				if breaksOutOfLoop(c.Body) {
					return true
				}
			}
			if breaksOutOfLoop(st.ElseBody) {
				return true
			}
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				if breaksOutOfLoop(c.Body) {
					return true
				}
			}
			if breaksOutOfLoop(st.ElseBody) {
				return true
			}
		}
	}
	return false
}

// endsPathDescription names the statement a dead one follows, for the message.
func endsPathDescription(s ast.MicroflowStatement) string {
	switch s.(type) {
	case *ast.IfStmt:
		return "an IF whose THEN and ELSE both end the flow"
	case *ast.WhileStmt:
		return "a `while true` that has no BREAK, so it is left only by return or an error"
	}
	return "a " + statementKindWord(s)
}

// unreachableSuggestion names the fix for a statement dead after s.
func unreachableSuggestion(s ast.MicroflowStatement) string {
	if _, ok := s.(*ast.WhileStmt); ok {
		return "Leave the loop with BREAK and keep the statement after END WHILE, or remove it"
	}
	return "Remove the statement, or move it before the " + statementKindWord(s)
}

// statementKindWord is a short MDL name for a statement kind, for messages.
func statementKindWord(s ast.MicroflowStatement) string {
	switch s.(type) {
	case *ast.ReturnStmt:
		return "RETURN"
	case *ast.RaiseErrorStmt:
		return "RAISE ERROR"
	case *ast.BreakStmt:
		return "BREAK"
	case *ast.ContinueStmt:
		return "CONTINUE"
	case *ast.JoinStmt:
		return "JOIN"
	case *ast.IfStmt:
		return "IF"
	case *ast.WhileStmt:
		return "WHILE"
	case *ast.LoopStmt:
		return "LOOP"
	case *ast.LogStmt:
		return "LOG"
	}
	return "statement"
}
