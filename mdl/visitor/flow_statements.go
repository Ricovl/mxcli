// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// FlowStatement is one statement of a microflow body, located in the text it
// was parsed from. It exists for code that rewrites a body as text and has to
// know where a statement ends and whether it already handles its own errors —
// the `mxcli test` generators, which attach an error handler to every call in
// a test body (ako/mxcli#1448). Searching the text for `on error` cannot answer
// that: the clause may sit on a later line, a custom handler holds statements
// of its own, and the words may be inside a string or a comment.
type FlowStatement struct {
	// Rule is the grammar rule the statement matched — the alternative of
	// microflowStatement, e.g. "callMicroflowStatement".
	Rule string
	// Start and Stop are the rune offsets [Start, Stop) of the statement,
	// without its annotations and without the `;` that terminates it.
	Start, Stop int
	// OnError is the statement's own ON ERROR clause, nil when it has none.
	// A clause on a statement inside a nested body (an IF branch, a loop, a
	// handler) belongs to that statement, which is listed separately.
	OnError *FlowOnError
}

// FlowOnError is an ON ERROR clause, located.
type FlowOnError struct {
	Type ast.ErrorHandlingType
	// Start and Stop are the rune offsets [Start, Stop) of the whole clause,
	// from `on` to `rollback`, `continue`, `end error` or the closing brace.
	Start, Stop int
}

// FlowStatements parses src as a script and returns every microflow-body
// statement in it, nested ones included, in source order. ok is false when src
// does not parse, and the caller must then leave the text alone.
//
// Offsets are in runes, as ANTLR counts them: index []rune(src), not src.
func FlowStatements(src string) (stmts []FlowStatement, ok bool) {
	errs := newErrorListener()
	lexer := parser.NewMDLLexer(newScriptStream(src, langver.V0))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	tree := p.Program()
	if len(errs.errors) > 0 {
		return nil, false
	}
	ruleNames := p.GetRuleNames()

	var walk func(t antlr.Tree)
	walk = func(t antlr.Tree) {
		if ms, isStmt := t.(*parser.MicroflowStatementContext); isStmt {
			if fs, found := flowStatement(ms, ruleNames); found {
				stmts = append(stmts, fs)
			}
		}
		for i := 0; i < t.GetChildCount(); i++ {
			walk(t.GetChild(i))
		}
	}
	walk(tree)
	return stmts, true
}

// flowStatement locates one microflowStatement: the statement rule inside it,
// past any annotations, and that rule's own ON ERROR clause.
func flowStatement(ms *parser.MicroflowStatementContext, ruleNames []string) (FlowStatement, bool) {
	for i := 0; i < ms.GetChildCount(); i++ {
		inner, isRule := ms.GetChild(i).(antlr.ParserRuleContext)
		if !isRule {
			continue
		}
		if _, isAnnotation := inner.(*parser.AnnotationContext); isAnnotation {
			continue
		}
		start, stop := inner.GetStart(), inner.GetStop()
		if start == nil || stop == nil {
			return FlowStatement{}, false
		}
		fs := FlowStatement{
			Rule:  ruleNames[inner.GetRuleIndex()],
			Start: start.GetStart(),
			Stop:  stop.GetStop() + 1,
		}
		if oe := ownOnErrorClause(inner); oe != nil {
			clause := buildOnErrorClause(oe)
			if clause != nil {
				fs.OnError = &FlowOnError{
					Type:  clause.Type,
					Start: oe.GetStart().GetStart(),
					Stop:  oe.GetStop().GetStop() + 1,
				}
			}
		}
		return fs, true
	}
	return FlowStatement{}, false
}

// ownOnErrorClause finds the ON ERROR clause that belongs to stmt itself. It
// does not descend into a nested body, whose statements own their clauses.
func ownOnErrorClause(t antlr.Tree) *parser.OnErrorClauseContext {
	for i := 0; i < t.GetChildCount(); i++ {
		switch c := t.GetChild(i).(type) {
		case *parser.OnErrorClauseContext:
			return c
		case *parser.MicroflowBodyContext:
			continue
		case antlr.ParserRuleContext:
			if found := ownOnErrorClause(c); found != nil {
				return found
			}
		}
	}
	return nil
}
