// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R3 (PROPOSAL_mdl_beta_syntax_freeze.md §3, ako/mxcli#751): `:` sets a model
// property. An `alter` sets properties in exactly the `( Key: value, … )` list
// its `create` takes, so a fragment of describe output pastes into an alter
// unchanged; a clause outside a property list takes no colon, and an attribute
// definition always has one. Every old spelling here is a respelling — the
// grammar builds the same statement from both — so each use is recorded as a
// deprecation with the edits that rewrite it, and nothing is gated on the
// language version.

// oldAssignment is one `Key = value` of an old-spelling list, by the rune
// offset where its key ends and the `=` token.
type oldAssignment struct {
	keyStop int
	op      antlr.Token
}

// assignmentAt reads an assignment whose key is child 0 of ctx and whose
// operator is the node op (a terminal or a one-token rule).
func assignmentAt(ctx antlr.ParserRuleContext, op antlr.Tree) (oldAssignment, bool) {
	if ctx == nil || op == nil || ctx.GetChildCount() == 0 {
		return oldAssignment{}, false
	}
	_, keyStop := nodeSpan(ctx.GetChild(0))
	var tok antlr.Token
	switch o := op.(type) {
	case antlr.TerminalNode:
		tok = o.GetSymbol()
	case antlr.ParserRuleContext:
		tok = o.GetStart()
	}
	if keyStop < 0 || tok == nil {
		return oldAssignment{}, false
	}
	return oldAssignment{keyStop: keyStop, op: tok}, true
}

// colonEdits writes each assignment's `=` as `:`, directly after the key:
// `Caption = 'x'` becomes `Caption: 'x'`.
//
// A comment between the key and the `=` stays where it is: only the `=` is
// replaced then (`Key /* c */: 'x'`).
func colonEdits(as []oldAssignment) []ast.TextEdit {
	out := make([]ast.TextEdit, 0, len(as))
	for _, a := range as {
		start := a.keyStop + 1
		if !blankBetween(a.op.GetInputStream(), start, a.op.GetStart()) {
			start = a.op.GetStart()
		}
		out = append(out, ast.TextEdit{Start: start, Stop: a.op.GetStop() + 1, Text: ":"})
	}
	return out
}

// blankBetween reports whether the runes in [start, stop) are whitespace only
// — no comment the rewrite would delete. A missing stream counts as blank.
func blankBetween(is antlr.CharStream, start, stop int) bool {
	if is == nil || stop <= start {
		return true
	}
	return strings.TrimSpace(is.GetText(start, stop-1)) == ""
}

// afterBlank is the offset of the first non-blank rune after tok: deleting
// [tok.start, afterBlank(tok)) removes the token and the space after it, and
// keeps a comment that follows.
func afterBlank(tok antlr.Token) int {
	end := tok.GetStop() + 1
	is := tok.GetInputStream()
	if is == nil {
		return end
	}
	for end < is.Size() {
		switch is.GetText(end, end) {
		case " ", "\t", "\n", "\r":
			end++
			continue
		}
		break
	}
	return end
}

// wrapEdits puts the list from first to last in parentheses. A list that
// starts on a new line after before (describe's layout) opens its parenthesis
// on before's line and closes it on a line of its own, indented as that line
// is; otherwise the parentheses hug the list, padded with a space when pad is
// set.
func wrapEdits(before, first, last antlr.Token, pad bool) []ast.TextEdit {
	if before != nil && first != nil && before.GetInputStream() != nil {
		is := before.GetInputStream()
		gap := is.GetText(before.GetStop()+1, first.GetStart()-1)
		if strings.Contains(gap, "\n") {
			return []ast.TextEdit{
				insertAt(before.GetStop()+1, " ("),
				insertAt(last.GetStop()+1, "\n"+lineIndentAt(is, before.GetStart())+")"),
			}
		}
	}
	open, closing := "(", ")"
	if pad {
		open, closing = "( ", " )"
	}
	return []ast.TextEdit{insertAt(first.GetStart(), open), insertAt(last.GetStop()+1, closing)}
}

// recordOldList records code for an old-spelling assignment list written
// after before, with the rewrite to `( Key: value, … )`.
func (b *Builder) recordOldList(code string, before antlr.Token, first, last antlr.ParserRuleContext, as []oldAssignment, subject string) {
	if first == nil || last == nil || first.GetStart() == nil || last.GetStop() == nil {
		return
	}
	b.recordDeprecation(code, first.GetStart(), subject)
	edits := append(colonEdits(as), wrapEdits(before, first.GetStart(), last.GetStop(), true)...)
	b.fixLastDeprecation(code, &ast.Fix{Edits: edits}, "")
}

// recordAlterPageSet reports the generic alter's old `set` spellings:
// `set Key = value` / `set (Key = value)` (MDL-DEPR101) and `set Key: value`
// (MDL-DEPR102). `=` is reported first: its rewrite, the parenthesised colon
// form, also adds a missing parenthesis.
func (b *Builder) recordAlterPageSet(ctx *parser.AlterSetContext) {
	assigns := ctx.AllAlterPageAssignment()
	if len(assigns) == 0 {
		return
	}
	var as []oldAssignment
	for _, a := range assigns {
		ac := a.(*parser.AlterPageAssignmentContext)
		if op, ok := ac.AlterAssignOp().(*parser.AlterAssignOpContext); ok && op != nil && op.EQUALS() != nil {
			if oa, ok := assignmentAt(ac, op); ok {
				as = append(as, oa)
			}
		}
	}
	var edits []ast.TextEdit
	code := deprecation.AlterPageSetEquals
	switch {
	case len(as) > 0:
		edits = colonEdits(as)
	case ctx.LPAREN() == nil:
		code = deprecation.AlterPageSetUnparenthesised
	default:
		return
	}
	if ctx.LPAREN() == nil {
		first := assigns[0].(*parser.AlterPageAssignmentContext).GetStart()
		last := assigns[len(assigns)-1].(*parser.AlterPageAssignmentContext).GetStop()
		edits = append(edits, insertAt(first.GetStart(), "("), insertAt(last.GetStop()+1, ")"))
	}
	b.recordDeprecation(code, ctx.SET().GetSymbol(), "alter set")
	b.fixLastDeprecation(code, &ast.Fix{Edits: edits}, "")
}

// recordAlterPageDropWidget reports `drop widget a, b` (MDL-DEPR103).
func (b *Builder) recordAlterPageDropWidget(ctx *parser.AlterDropContext) {
	w := ctx.WIDGET()
	targets := ctx.AllAlterTarget()
	if w == nil || len(targets) == 0 {
		return
	}
	tok := w.GetSymbol()
	b.recordDeprecation(deprecation.AlterPageDropWidget, tok, "drop widget")
	edit := ast.TextEdit{Start: tok.GetStart(), Stop: afterBlank(tok), Text: ""}
	b.fixLastDeprecation(deprecation.AlterPageDropWidget, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// recordSettingsAssignments reports a settings list written `Key = value, …`
// (MDL-DEPR060), in `alter settings` and `create configuration`.
func (b *Builder) recordSettingsAssignments(assigns []parser.ISettingsAssignmentContext) {
	if len(assigns) == 0 {
		return
	}
	var as []oldAssignment
	for _, a := range assigns {
		ac := a.(*parser.SettingsAssignmentContext)
		if oa, ok := assignmentAt(ac, ac.EQUALS()); ok {
			as = append(as, oa)
		}
	}
	first := assigns[0].(*parser.SettingsAssignmentContext)
	b.recordOldList(deprecation.SettingsAssignment, tokenBefore(first), first,
		assigns[len(assigns)-1].(*parser.SettingsAssignmentContext), as, "settings")
}

// recordODataAlterAssignments reports `alter … odata service X set Key =
// value, …` (MDL-DEPR061).
func (b *Builder) recordODataAlterAssignments(ctx *parser.AlterStatementContext) {
	assigns := ctx.AllOdataAlterAssignment()
	if len(assigns) == 0 {
		return
	}
	var as []oldAssignment
	for _, a := range assigns {
		ac := a.(*parser.OdataAlterAssignmentContext)
		if oa, ok := assignmentAt(ac, ac.EQUALS()); ok {
			as = append(as, oa)
		}
	}
	b.recordOldList(deprecation.ODataAlterAssignment, ctx.SET().GetSymbol(),
		assigns[0].(*parser.OdataAlterAssignmentContext), assigns[len(assigns)-1].(*parser.OdataAlterAssignmentContext),
		as, "odata service")
}

// ExitAlterStylingAction reports `alter styling … set Class = 'x', 'P' = on`
// (MDL-DEPR062): an `=`, or a list without its parentheses.
func (b *Builder) ExitAlterStylingAction(ctx *parser.AlterStylingActionContext) {
	assigns := ctx.AllAlterStylingAssignment()
	if ctx.SET() == nil || len(assigns) == 0 {
		return
	}
	var as []oldAssignment
	for _, a := range assigns {
		ac := a.(*parser.AlterStylingAssignmentContext)
		if op, ok := ac.AlterStylingAssignOp().(*parser.AlterStylingAssignOpContext); ok && op != nil && op.EQUALS() != nil {
			if oa, ok := assignmentAt(ac, op); ok {
				as = append(as, oa)
			}
		}
	}
	parenthesised := ctx.LPAREN() != nil
	if len(as) == 0 && parenthesised {
		return
	}
	edits := colonEdits(as)
	first := assigns[0].(*parser.AlterStylingAssignmentContext)
	last := assigns[len(assigns)-1].(*parser.AlterStylingAssignmentContext)
	if !parenthesised {
		edits = append(edits, wrapEdits(ctx.SET().GetSymbol(), first.GetStart(), last.GetStop(), true)...)
	}
	b.recordDeprecation(deprecation.StylingAssignment, ctx.SET().GetSymbol(), "alter styling")
	b.fixLastDeprecation(deprecation.StylingAssignment, &ast.Fix{Edits: edits}, "")
}

// recordAllowCreateChangeLocally reports `set allow_create_change_locally =
// v` (MDL-DEPR063), rewritten to `set ( AllowCreateChangeLocally: v )`.
func (b *Builder) recordAllowCreateChangeLocally(ctx *parser.AlterEntityActionContext) {
	key, eq := ctx.ALLOW_CREATE_CHANGE_LOCALLY(), ctx.EQUALS()
	if ctx.SET() == nil || key == nil || eq == nil {
		return
	}
	var value antlr.TerminalNode = ctx.TRUE()
	if value == nil {
		value = ctx.FALSE()
	}
	if value == nil {
		return
	}
	kt := key.GetSymbol()
	b.recordDeprecation(deprecation.AllowCreateChangeLocally, kt, "alter entity")
	b.fixLastDeprecation(deprecation.AllowCreateChangeLocally, &ast.Fix{Edits: []ast.TextEdit{
		{Start: kt.GetStart(), Stop: kt.GetStop() + 1, Text: "( AllowCreateChangeLocally"},
		colonEdits([]oldAssignment{{keyStop: kt.GetStop(), op: eq.GetSymbol()}})[0],
		insertAt(value.GetSymbol().GetStop()+1, " )"),
	}}, "")
}

// recordModifyAttributeColon reports `modify attribute A Type` (MDL-DEPR065),
// rewritten to `modify attribute A: Type`.
func (b *Builder) recordModifyAttributeColon(ctx *parser.AlterEntityActionContext) {
	if ctx.MODIFY() == nil || ctx.COLON() != nil || ctx.DataType() == nil {
		return
	}
	names := ctx.AllAttributeName()
	if len(names) == 0 || names[0].GetStop() == nil {
		return
	}
	stop := names[0].GetStop()
	b.recordDeprecation(deprecation.ModifyAttributeColon, stop, "modify attribute")
	b.fixLastDeprecation(deprecation.ModifyAttributeColon,
		&ast.Fix{Edits: []ast.TextEdit{insertAt(stop.GetStop()+1, ":")}}, "")
}

// recordAssociationClauseColon reports `type: Reference`, `owner: Both` and
// `storage: Table` (MDL-DEPR064): the colon goes, and the clause keeps one
// space before its value.
func (b *Builder) recordAssociationClauseColon(ctx *parser.AssociationOptionContext) {
	colon := ctx.COLON()
	if colon == nil || ctx.GetChildCount() < 3 {
		return
	}
	_, kwStop := nodeSpan(ctx.GetChild(0))
	valueStart, _ := nodeSpan(ctx.GetChild(2))
	if kwStop < 0 || valueStart < 0 {
		return
	}
	ct := colon.GetSymbol()
	is := ct.GetInputStream()
	edit := replaceGap(kwStop, valueStart, " ")
	if !blankBetween(is, kwStop+1, valueStart) {
		// A comment sits in the gap: delete only the colon and the space
		// after it, keeping one space between the keyword and what follows.
		text := ""
		if kwStop+1 == ct.GetStart() {
			text = " "
		}
		edit = ast.TextEdit{Start: ct.GetStart(), Stop: afterBlank(ct), Text: text}
	}
	b.recordDeprecation(deprecation.AssociationClauseColon, ct, "association")
	b.fixLastDeprecation(deprecation.AssociationClauseColon, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// lineIndentAt is the leading whitespace of the line holding rune offset pos.
func lineIndentAt(is antlr.CharStream, pos int) string {
	if pos <= 0 {
		return ""
	}
	head := is.GetText(0, pos-1)
	line := head[strings.LastIndex(head, "\n")+1:]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// tokenBefore is the token directly before ctx in the token stream (on the
// default channel), or nil.
func tokenBefore(ctx antlr.ParserRuleContext) antlr.Token {
	p, ok := ctx.GetParent().(antlr.ParserRuleContext)
	if !ok || p == nil {
		return nil
	}
	var prev antlr.Token
	for i := 0; i < p.GetChildCount(); i++ {
		c := p.GetChild(i)
		if c == antlr.Tree(ctx) {
			return prev
		}
		switch x := c.(type) {
		case antlr.TerminalNode:
			prev = x.GetSymbol()
		case antlr.ParserRuleContext:
			if x.GetStop() != nil {
				prev = x.GetStop()
			}
		}
	}
	return nil
}
