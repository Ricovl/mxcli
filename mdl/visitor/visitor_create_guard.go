// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// EnterCreateStatement remembers how many statements were built before this
// one, so ExitCreateStatement can find the statement the create rule built.
func (b *Builder) EnterCreateStatement(ctx *parser.CreateStatementContext) {
	b.createStart = len(b.statements)
}

// applyCreateGuard applies `if not exists` to whichever statement the create
// rule built (ako/mxcli#731, ADR-0010 R1).
//
// The guard is written in each create rule — after the kind's keywords, before
// the name — but applied here once, the way `drop … if exists` is applied in
// ExitDropStatement: a guard each document builder has to remember to read is
// the one the next document type forgets. A statement type that does not embed
// ast.CreateGuard is reported rather than silently built without the guard,
// since an ignored guard turns a leave-it-alone create into a plain one.
func (b *Builder) applyCreateGuard(ctx *parser.CreateStatementContext) {
	guard := createGuardToken(ctx)
	if guard == nil || len(b.statements) != b.createStart+1 {
		return
	}
	stmt := b.statements[len(b.statements)-1]
	g, ok := stmt.(ast.IfNotExistsCreate)
	if !ok {
		b.addError(fmt.Errorf("line %d: `if not exists` is not supported on %T",
			guard.GetStart().GetLine(), stmt))
		return
	}
	g.SetCreateIfNotExists(ctx.OR() != nil)
}

// createGuardToken returns the `if not exists` of the create rule under ctx, or
// nil. Only the create rule's own children are searched: a guard deeper in the
// tree belongs to something inside the document, not to the document.
func createGuardToken(ctx *parser.CreateStatementContext) *parser.IfNotExistsContext {
	for _, child := range ctx.GetChildren() {
		rc, ok := child.(antlr.ParserRuleContext)
		if !ok {
			continue
		}
		for _, grand := range rc.GetChildren() {
			if g, ok := grand.(*parser.IfNotExistsContext); ok {
				return g
			}
		}
	}
	return nil
}
