// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R9 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ADR-0010): where document
// metadata lives. Documentation is a `/** … */` doc comment before the
// statement, and the folder a `folder '…'` clause after the name, on every
// document. The older places — a `comment '…'` clause, a `Documentation:` or
// `Folder:` property — are registered aliases: they build the same statement,
// the listeners below record them, and the rewrite moves the text to its
// canonical place.

// recordDocumentationClause records a `comment '…'` clause (MDL-DEPR100) that
// spans kw to str inside ctx, with the rewrite that deletes it and writes its
// text as a doc comment before the statement. wins says which of the two
// spellings the statement's documentation is when it has both.
func (b *Builder) recordDocumentationClause(ctx antlr.RuleContext, kw, str antlr.Token, text string, wins bool) {
	if kw == nil || str == nil {
		return
	}
	b.recordDeprecation(deprecation.DocumentationClause, kw, "")
	del := ast.TextEdit{Start: startAfterSpace(kw), Stop: str.GetStop() + 1}
	fix, reason := docCommentFix(ctx, del, text, wins)
	b.fixLastDeprecation(deprecation.DocumentationClause, fix, reason)
}

// recordDocumentationProperty records a `Documentation: '…'` property
// (MDL-DEPR106), item i of a property list, with the rewrite that deletes it
// from the list and writes its text as a doc comment before the statement.
// The property overrides a doc comment when a statement has both.
func (b *Builder) recordDocumentationProperty(ctx antlr.RuleContext, items []antlr.ParserRuleContext, i int, text string) {
	b.recordDeprecation(deprecation.DocumentationProperty, items[i].GetStart(), "")
	del, reason := deleteListItem(items, i, nil)
	if reason != "" {
		b.fixLastDeprecation(deprecation.DocumentationProperty, nil, reason)
		return
	}
	fix, reason := docCommentFix(ctx, del, text, true)
	b.fixLastDeprecation(deprecation.DocumentationProperty, fix, reason)
}

// recordFolderProperty records a `Folder: '…'` property (MDL-DEPR105), item i
// of a property list, with the rewrite that deletes it from the list and
// writes `folder '…'` after name. clause reports that the statement also has
// the clause, which leaves the two to be reconciled by hand. list is the whole
// bracketed list when it is optional, so an only item goes with its brackets.
func (b *Builder) recordFolderProperty(name antlr.ParserRuleContext, items []antlr.ParserRuleContext, i int,
	folder string, clause bool, list antlr.ParserRuleContext) {
	key := items[i].GetStart()
	b.recordDeprecation(deprecation.FolderProperty, key, "")
	if clause {
		b.fixLastDeprecation(deprecation.FolderProperty, nil,
			"the statement has both a `folder` clause and a `Folder:` property; delete one by hand")
		return
	}
	lit, ok := stringLiteral(folder)
	if !ok {
		b.fixLastDeprecation(deprecation.FolderProperty, nil,
			"the folder name has a backslash, which a string literal reads differently under each language version")
		return
	}
	del, reason := deleteListItem(items, i, list)
	if reason != "" {
		b.fixLastDeprecation(deprecation.FolderProperty, nil, reason)
		return
	}
	// The clause is a keyword, spelt in the case of the statement's own
	// keywords; the property's key is usually `Folder`, which says nothing.
	like := key.GetText()
	if c := findParentCreateStatement(name); c != nil && c.CREATE() != nil {
		like = c.CREATE().GetText()
	}
	ins := insertAt(name.GetStop().GetStop()+1, " "+keywordLike(like, "folder")+" "+lit)
	b.fixLastDeprecation(deprecation.FolderProperty, &ast.Fix{Edits: []ast.TextEdit{ins, del}}, "")
}

// docCommentFix is the rewrite that applies del and writes text as a doc
// comment before the statement ctx is part of, or the reason there is none.
//
// A statement that already has a doc comment keeps the meaning it has: when
// the alias wins (wins), the doc comment was never stored, so it is demoted to
// a plain `/* … */` comment — its text stays in the script — and the alias's
// text becomes the doc comment; when the doc comment wins, the alias was never
// stored, and is deleted.
func docCommentFix(ctx antlr.RuleContext, del ast.TextEdit, text string, wins bool) (*ast.Fix, string) {
	stmt := findParentStatement(ctx)
	if stmt == nil || stmt.GetStart() == nil {
		return nil, "the statement could not be located"
	}
	if doc := docCommentToken(ctx); doc != nil {
		if !wins {
			return &ast.Fix{Edits: []ast.TextEdit{del}}, ""
		}
		if strings.HasPrefix(doc.GetText(), "/***") {
			return nil, "the statement also has a `/*** … */` doc comment, which cannot be demoted to a plain " +
				"comment by removing one `*`; merge the two by hand"
		}
		if !docCommentHolds(text) {
			return nil, "a doc comment cannot hold this text exactly (a `*/`, a blank line, or space at the " +
				"start or end of a line); write the doc comment by hand"
		}
		demote := ast.TextEdit{Start: doc.GetStart(), Stop: doc.GetStart() + 3, Text: "/*"}
		ins := insertAt(doc.GetStop()+1, "\n"+lineIndent(doc)+"/** "+text+" */")
		return &ast.Fix{Edits: []ast.TextEdit{demote, ins, del}}, ""
	}
	if !docCommentHolds(text) {
		return nil, "a doc comment cannot hold this text exactly (a `*/`, a blank line, or space at the " +
			"start or end of a line); write the doc comment by hand"
	}
	start := stmt.GetStart()
	sep := "\n" + lineIndent(start)
	if !atLineStart(start) {
		sep = " "
	}
	ins := insertAt(start.GetStart(), "/** "+text+" */"+sep)
	return &ast.Fix{Edits: []ast.TextEdit{ins, del}}, ""
}

// docCommentToken is the doc comment of the statement ctx is part of, found
// where findDocComment finds it, or nil.
func docCommentToken(ctx antlr.RuleContext) antlr.Token {
	if c := findParentCreateStatement(ctx); c != nil && c.DocComment() != nil {
		return c.DocComment().GetStart()
	}
	if s := findParentStatement(ctx); s != nil && s.DocComment() != nil {
		return s.DocComment().GetStart()
	}
	return nil
}

// docCommentHolds reports whether a `/** text */` doc comment reads back as
// exactly text: extractDocComment trims each line and drops blank ones.
func docCommentHolds(text string) bool {
	return !strings.Contains(text, "*/") && extractDocComment("/** "+text+" */") == text
}

// stringLiteral quotes s as an MDL string literal that reads back as s under
// every language version, or reports that there is none: a backslash is an
// escape under mdl 0 and a character under mdl 1.
func stringLiteral(s string) (string, bool) {
	if strings.Contains(s, `\`) {
		return "", false
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
}

// deleteListItem is the edit that deletes item i of a comma-separated list,
// with the comma that separates it from its neighbour, or the reason there is
// none. An only item goes with the brackets when the list is optional (list is
// the bracketed list), and cannot go when it is not.
func deleteListItem(items []antlr.ParserRuleContext, i int, list antlr.ParserRuleContext) (ast.TextEdit, string) {
	switch {
	case i+1 < len(items):
		return ast.TextEdit{Start: items[i].GetStart().GetStart(), Stop: items[i+1].GetStart().GetStart()}, ""
	case i > 0:
		return ast.TextEdit{Start: items[i-1].GetStop().GetStop() + 1, Stop: items[i].GetStop().GetStop() + 1}, ""
	case list != nil:
		return ast.TextEdit{Start: startAfterSpace(list.GetStart()), Stop: list.GetStop().GetStop() + 1}, ""
	}
	return ast.TextEdit{}, "it is the only property in the list, which may not be empty"
}

// startAfterSpace is the offset of tok, moved back over the whitespace before
// it, so deleting from there removes the clause and the gap that set it off.
func startAfterSpace(tok antlr.Token) int {
	in := tok.GetInputStream()
	i := tok.GetStart()
	for i > 0 && isBlank(in.GetText(i-1, i-1)) {
		i--
	}
	return i
}

func isBlank(c string) bool { return c == " " || c == "\t" || c == "\r" || c == "\n" }

// lineIndent is the run of spaces and tabs right before tok.
func lineIndent(tok antlr.Token) string {
	in := tok.GetInputStream()
	i := tok.GetStart()
	for i > 0 {
		if c := in.GetText(i-1, i-1); c != " " && c != "\t" {
			break
		}
		i--
	}
	if i == tok.GetStart() {
		return ""
	}
	return in.GetText(i, tok.GetStart()-1)
}

// atLineStart reports whether only spaces and tabs precede tok on its line.
func atLineStart(tok antlr.Token) bool {
	in := tok.GetInputStream()
	for i := tok.GetStart(); i > 0; i-- {
		switch in.GetText(i-1, i-1) {
		case " ", "\t":
			continue
		case "\n":
			return true
		}
		return false
	}
	return true
}

// ruleContexts converts a generated list of rule contexts to the plain
// interface the list helpers take.
func ruleContexts[T antlr.ParserRuleContext](items []T) []antlr.ParserRuleContext {
	out := make([]antlr.ParserRuleContext, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

// recordFolderClausePosition records a `folder '…'` clause written after the
// rest of the statement (MDL-DEPR134), with the rewrite that moves it to right
// after name. twice reports that the statement also states the folder
// elsewhere, which leaves the two to be reconciled by hand.
func (b *Builder) recordFolderClausePosition(name antlr.ParserRuleContext, kw, lit antlr.Token, twice bool) {
	b.recordDeprecation(deprecation.FolderClausePosition, kw, "")
	if twice || name == nil || name.GetStop() == nil {
		b.fixLastDeprecation(deprecation.FolderClausePosition, nil,
			"the statement states its folder more than once; keep one `folder '…'` right after the name by hand")
		return
	}
	del := ast.TextEdit{Start: startAfterSpace(kw), Stop: lit.GetStop() + 1}
	ins := insertAt(name.GetStop().GetStop()+1, " "+kw.GetText()+" "+lit.GetText())
	b.fixLastDeprecation(deprecation.FolderClausePosition, &ast.Fix{Edits: []ast.TextEdit{ins, del}}, "")
}

// countFolderOptions counts the `folder` options among a constant's options.
func countFolderOptions(opts *parser.ConstantOptionsContext) int {
	n := 0
	for _, o := range opts.AllConstantOption() {
		if o.(*parser.ConstantOptionContext).FOLDER() != nil {
			n++
		}
	}
	return n
}

// snippetHeaderHasFolder reports whether a snippet's header has a `Folder:`
// property (MDL-DEPR105's alias of the same clause).
func snippetHeaderHasFolder(ctx *parser.CreateSnippetStatementContext) bool {
	h, ok := ctx.SnippetHeaderV3().(*parser.SnippetHeaderV3Context)
	if !ok || h == nil {
		return false
	}
	for _, p := range h.AllSnippetHeaderPropertyV3() {
		if p.(*parser.SnippetHeaderPropertyV3Context).FOLDER() != nil {
			return true
		}
	}
	return false
}
