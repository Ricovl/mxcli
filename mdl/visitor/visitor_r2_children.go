// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R2 (ADR-0010, ako/mxcli#754): `( Key: value, … )` holds an element's
// properties and `{ … }` its declarative children, each shaped
// `<kind> [Name] ( props ) [ { children } ]`. The integration documents had the
// brackets the other way round:
//
//   - a REST client operation and an agent's tool, MCP service and knowledge
//     base put their properties in braces (MDL-DEPR070, MDL-DEPR071);
//   - an image collection put its images in parentheses, each written
//     `image X from file '…'` (MDL-DEPR072);
//   - a message definition collection put its definitions, and every member
//     tree, in parentheses (MDL-DEPR073);
//   - an `alter microflow` fragment, which is imperative flow, was in braces
//     rather than `begin … end` (MDL-DEPR074).
//
// Each old form is a respelling that builds the same statement, so it keeps
// parsing under every language version, warns, and `fmt --upgrade` rewrites it.

// EnterStatement records each of the four codes at most once per statement:
// a service with twenty operations in braces is one warning, not twenty. The
// rewrite of that one record respells every old bracket pair in the statement.
func (b *Builder) EnterStatement(ctx *parser.StatementContext) {
	b.enterStatementDocs(ctx)
	uses := map[string]*r2Use{}
	var order []string
	use := func(code string, at antlr.Token) *r2Use {
		u := uses[code]
		if u == nil {
			u = &r2Use{first: at}
			uses[code] = u
			order = append(order, code)
		}
		return u
	}
	var walk func(antlr.Tree)
	walk = func(n antlr.Tree) {
		switch x := n.(type) {
		case *parser.RestClientOperationContext:
			if x.LBRACE() != nil && x.RBRACE() != nil {
				use(deprecation.RestOperationBraces, x.LBRACE().GetSymbol()).swap(x.LBRACE(), x.RBRACE(), "(", ")")
			}
		case *parser.AgentBodyBlockContext:
			if x.LBRACE() != nil && x.RBRACE() != nil {
				use(deprecation.AgentAttachmentBraces, x.LBRACE().GetSymbol()).swap(x.LBRACE(), x.RBRACE(), "(", ")")
			}
		case *parser.ImageCollectionBodyContext:
			if x.LPAREN() != nil && x.RPAREN() != nil {
				use(deprecation.ImageCollectionParens, x.LPAREN().GetSymbol()).imageItems(x)
			}
		case *parser.CreateMessageDefinitionCollectionStatementContext:
			if x.LPAREN() != nil && x.RPAREN() != nil {
				use(deprecation.MessageTreeParens, x.LPAREN().GetSymbol()).swap(x.LPAREN(), x.RPAREN(), "{", "}")
			}
		case *parser.AlterFlowFragmentContext:
			if x.LBRACE() != nil && x.RBRACE() != nil {
				// In the case of the operation's verb: `INSERT … BEGIN … END`.
				like := "insert"
				if op, ok := x.GetParent().(antlr.ParserRuleContext); ok && op.GetStart() != nil {
					like = op.GetStart().GetText()
				}
				use(deprecation.AlterFlowFragmentBraces, x.LBRACE().GetSymbol()).swapWords(x.LBRACE(), x.RBRACE(),
					keywordLike(like, "begin"), keywordLike(like, "end"))
			}
		case *parser.MessageMemberTreeContext:
			if x.LPAREN() != nil && x.RPAREN() != nil {
				use(deprecation.MessageTreeParens, x.LPAREN().GetSymbol()).swap(x.LPAREN(), x.RPAREN(), "{", "}")
			}
		}
		r2RestUse(n, use)
		for _, c := range n.GetChildren() {
			walk(c)
		}
	}
	walk(ctx)
	for _, code := range order {
		u := uses[code]
		b.recordDeprecation(code, u.first, "")
		b.fixLastDeprecation(code, &ast.Fix{Edits: u.edits}, "")
	}
}

// r2Use is the uses of one code in one statement: where the first is, and the
// edits that rewrite all of them.
type r2Use struct {
	first antlr.Token
	edits []ast.TextEdit
}

// swap respells the bracket pair open … closing as newOpen … newClose.
func (u *r2Use) swap(open, closing antlr.TerminalNode, newOpen, newClose string) {
	o, c := open.GetSymbol(), closing.GetSymbol()
	u.edits = append(u.edits, replaceSpan(o, o, newOpen), replaceSpan(c, c, newClose))
}

// swapWords respells a brace pair as the words newOpen … newClose. A brace is
// punctuation and may touch its neighbours (`$X{ … }drop`); a word may not, or
// it lexes as part of them (`$Xbegin`, `enddrop`), so each word is set apart
// by a space where the source has none.
func (u *r2Use) swapWords(open, closing antlr.TerminalNode, newOpen, newClose string) {
	o, c := open.GetSymbol(), closing.GetSymbol()
	u.edits = append(u.edits,
		replaceSpan(o, o, padWord(o, newOpen)),
		replaceSpan(c, c, padWord(c, newClose)))
}

// padWord is word, with a space on each side where the text next to tok is
// not whitespace. A `;` after it needs none: `end;` is the canonical spelling.
func padWord(tok antlr.Token, word string) string {
	in := tok.GetInputStream()
	if in == nil {
		return word
	}
	if start := tok.GetStart(); start > 0 && !isSpaceText(in.GetText(start-1, start-1)) {
		word = " " + word
	}
	if stop := tok.GetStop(); stop+1 < in.Size() {
		if next := in.GetText(stop+1, stop+1); next != ";" && !isSpaceText(next) {
			word += " "
		}
	}
	return word
}

func isSpaceText(s string) bool {
	return s == "" || strings.TrimSpace(s) == ""
}

// imageItems rewrites `( image X from file '…', … )` as
// `{ image X ( File: '…' ) … }`.
func (u *r2Use) imageItems(ctx *parser.ImageCollectionBodyContext) {
	u.swap(ctx.LPAREN(), ctx.RPAREN(), "{", "}")
	for _, c := range ctx.AllCOMMA() {
		u.edits = append(u.edits, replaceSpan(c.GetSymbol(), c.GetSymbol(), ""))
	}
	for _, it := range ctx.AllImageCollectionItem() {
		item, ok := it.(*parser.ImageCollectionItemContext)
		if !ok || item.FROM() == nil || item.GetPath() == nil {
			continue
		}
		path := item.GetPath()
		u.edits = append(u.edits, replaceSpan(item.FROM().GetSymbol(), path, "( File: "+path.GetText()+" )"))
	}
}
