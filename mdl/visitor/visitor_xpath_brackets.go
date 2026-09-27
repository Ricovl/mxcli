// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R5 (ADR-0010; ako/mxcli#753): an XPath is always written in [ ], never in a
// string. The entity grant's `where` and a workflow user task's `targeting
// xpath` took a string, so every quote inside the XPath was doubled — six in a
// row for a token comparison (#750). Both now take one or more bracketed
// predicate groups, kept verbatim: nothing inside them was escaped, so nothing
// is unescaped. The quoted spellings are deprecated aliases (MDL-DEPR030,
// MDL-DEPR031) whose rewrite takes the XPath out of its string.

// bracketedXPathText is the source text of a run of xpathConstraint groups,
// from the first `[` to the last `]`, exactly as written. Mendix stores sibling
// groups concatenated (`[a][b]`), so the run is one constraint.
func bracketedXPathText(groups []parser.IXpathConstraintContext) string {
	if len(groups) == 0 {
		return ""
	}
	first, last := groups[0].GetStart(), groups[len(groups)-1].GetStop()
	if first == nil || last == nil {
		return ""
	}
	is := first.GetInputStream()
	if is == nil || last.GetStop() < first.GetStart() {
		return ""
	}
	return is.GetText(first.GetStart(), last.GetStop())
}

// IsBracketedXPath reports whether s, written after `where` or `xpath`, parses
// as one or more bracketed XPath predicate groups and nothing else — not even
// surrounding whitespace, which the bracketed form could not carry. Describe
// asks it before writing a stored constraint in [ ]; a stored value that does
// not parse is written in the deprecated quoted form instead, which keeps the
// output re-executable. The rewrite of the quoted form asks it too.
func IsBracketedXPath(s string) bool {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return false
	}
	lexer := parser.NewMDLLexer(antlr.NewInputStream(s))
	lexer.RemoveErrorListeners()
	errs := &countingErrorListener{}
	lexer.AddErrorListener(errs)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	groups := 0
	for stream.LA(1) == parser.MDLParserLBRACKET {
		p.XpathConstraint()
		groups++
		if errs.n > 0 {
			return false
		}
	}
	return groups > 0 && errs.n == 0 && stream.LA(1) == antlr.TokenEOF
}

type countingErrorListener struct {
	*antlr.DefaultErrorListener
	n int
}

func (l *countingErrorListener) SyntaxError(antlr.Recognizer, any, int, int, string, antlr.RecognitionException) {
	l.n++
}

// quotedXPathFix is the rewrite of a quoted XPath to the bracketed form: the
// string literal is replaced by its value. It has none when the value is not
// a bracketed XPath (the brackets are the syntax now, so a value without them
// would change what is stored), or when the literal holds an mdl 0 escape,
// which the string-escape rewrite edits in place.
func quotedXPathFix(lit antlr.TerminalNode) ([]ast.TextEdit, string) {
	if holdsInterpretedEscape(lit) {
		return nil, "the XPath string holds a backslash escape; write the XPath in [ ] by hand"
	}
	value := unquoteStringLit(lit)
	if !IsBracketedXPath(value) {
		return nil, "the XPath string's value " + lit.GetText() + " is not one or more [ ] predicate groups; " +
			"write it in [ ] by hand"
	}
	t := lit.GetSymbol()
	return []ast.TextEdit{{Start: t.GetStart(), Stop: t.GetStop() + 1, Text: value}}, ""
}

// recordReversedEntityGrant records MDL-DEPR030 on the reversed grant and
// computes its rewrite: `grant R1, R2 on M.E (rights) [where '…']` becomes
// `grant rights on entity M.E to R1, R2 [where […]]`. Roles, entity and rights
// are moved as written; only the keywords between them and the XPath's
// quoting change.
func (b *Builder) recordReversedEntityGrant(ctx *parser.GrantEntityAccessStatementContext) {
	grant := ctx.GRANT().GetSymbol()
	b.recordDeprecation(deprecation.ReversedEntityGrant, grant, "")

	roles, ok1 := ctx.ModuleRoleList().(antlr.ParserRuleContext)
	entity, ok2 := ctx.QualifiedName().(antlr.ParserRuleContext)
	rights, ok3 := ctx.EntityAccessRightList().(antlr.ParserRuleContext)
	rparen := ctx.RPAREN()
	if !ok1 || !ok2 || !ok3 || rparen == nil {
		b.fixLastDeprecation(deprecation.ReversedEntityGrant, nil, "the statement is incomplete")
		return
	}
	like := grant.GetText()
	head := nodeText(rights) + " " + keywordLike(like, "on") + " " + keywordLike(like, "entity") + " " +
		nodeText(entity) + " " + keywordLike(like, "to") + " " + nodeText(roles)
	edits := []ast.TextEdit{replaceSpan(roles.GetStart(), rparen.GetSymbol(), head)}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		xp, why := quotedXPathFix(lit)
		if why != "" {
			b.fixLastDeprecation(deprecation.ReversedEntityGrant, nil, why)
			return
		}
		edits = append(edits, xp...)
	}
	b.fixLastDeprecation(deprecation.ReversedEntityGrant, &ast.Fix{Edits: edits}, "")
}

// recordQuotedTargetingXPath records MDL-DEPR031 on a quoted targeting XPath,
// with the rewrite that takes the XPath out of its string.
func (b *Builder) recordQuotedTargetingXPath(lit antlr.TerminalNode) {
	if lit == nil {
		return
	}
	b.recordDeprecation(deprecation.QuotedTargetingXPath, lit.GetSymbol(), "")
	fix, why := fixOrReason(quotedXPathFix(lit))
	b.fixLastDeprecation(deprecation.QuotedTargetingXPath, fix, why)
}

// ExitWorkflowUserTaskClause records a user task's quoted `targeting xpath`.
// The clause is folded into the task by a builder with no Builder at hand, so
// the use is recorded here, from the listener.
func (b *Builder) ExitWorkflowUserTaskClause(ctx *parser.WorkflowUserTaskClauseContext) {
	if ctx.TARGETING() != nil && ctx.XPATH() != nil {
		b.recordQuotedTargetingXPath(ctx.STRING_LITERAL())
	}
}

// ExitActivitySetProperty records `alter workflow … set activity … targeting
// xpath '…'`, for the same reason.
func (b *Builder) ExitActivitySetProperty(ctx *parser.ActivitySetPropertyContext) {
	if ctx.TARGETING() != nil && ctx.XPATH() != nil {
		b.recordQuotedTargetingXPath(ctx.STRING_LITERAL())
	}
}
