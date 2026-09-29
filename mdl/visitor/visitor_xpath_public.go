// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ParseXPathConstraint parses a raw XPath constraint string — including the outer
// [ ] brackets stored by Mendix in the XPathConstraint BSON field — and returns the
// AST expression. Returns (nil, false) if the input cannot be parsed (e.g. empty,
// malformed, or not starting with '[').
//
// The rule matches a SINGLE bracket group. Mendix stores sibling groups
// concatenated — `[a][b][c]` — and with the error listeners removed ANTLR happily
// parsed the first and left the rest on the stream, returning true. Callers read
// that as "fully parsed" and re-rendered only what came back, silently dropping
// every later group (mendixlabs/mxcli#772). A partial parse is therefore reported
// as a failure so callers fall back to the untouched string; use
// SplitXPathPredicateGroups to handle each group in turn.
//
// The input is a STORED constraint, spelled the way Mendix spells it — a
// doubled apostrophe is the only escape in a string and a backslash is itself —
// whatever language the script that wrote it is in, so it is read with that
// rule (storedXPathStream). Read with the mdl 0 lexer, `'C:\temp'` held a tab
// and `'C:\'` ran on into the rest of the constraint; the executor re-derives
// the layout of every long or multi-line constraint from this parse, so that
// was stored (ako/mxcli#825).
func ParseXPathConstraint(input string) (ast.Expression, bool) {
	if input == "" {
		return nil, false
	}

	lexer := parser.NewMDLLexer(storedXPathStream(input))
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()

	ctx := p.XpathConstraint()
	xcCtx, ok := ctx.(*parser.XpathConstraintContext)
	if !ok {
		return nil, false
	}
	xpathExpr := xcCtx.XpathExpr()
	if xpathExpr == nil {
		return nil, false
	}
	// Anything left on the stream means the rule consumed only a prefix.
	if stream.LA(1) != antlr.TokenEOF {
		return nil, false
	}
	return buildXPathExpr(xpathExpr), true
}

// storedXPathStream is the character stream a stored constraint is read
// from: with the string rule of a Mendix XPath, which is ADR-0010 R11's.
func storedXPathStream(input string) antlr.CharStream {
	return &parser.StrictEscapeStream{CharStream: antlr.NewInputStream(input)}
}
