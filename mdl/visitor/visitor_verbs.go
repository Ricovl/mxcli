// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// R6 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ADR-0010): one verb per job.
// `show` is split between `list` and `describe`, an alter's children are
// added and dropped, and the words that broke a pattern (`rest call`,
// `define fragment`, `describe widget <name>`, the `column` synonym) take the
// pattern's word. Every old spelling builds the same statement as its
// replacement, so the parse tree is the only place it is visible: the
// listeners below record it (codes MDL-DEPR090..096).

// recordShowSingleThing records MDL-DEPR090 for `show` (or `list`) on a form
// that names one thing whose describe is the same statement: page, app
// security, security matrix, structure and context. Each use carries the
// rewrite to `describe`.
func (b *Builder) recordShowSingleThing(ctx *parser.ShowOrListContext) {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok {
		return
	}
	first := tokenTypeAt(stmt, 1)
	if first == parser.MDLParserPROJECT && stmt.SECURITY() == nil {
		// `show project version`: not a statement. The parser has already
		// reported the syntax error; its recovered tree has no SECURITY token
		// to rewrite up to, and there is nothing to deprecate.
		return
	}
	verb := ctx.GetStart()
	b.recordDeprecation(deprecation.ShowSingleThing, verb, "")
	edit := replaceSpan(verb, verb, keywordLike(verb.GetText(), "describe"))
	if first == parser.MDLParserPROJECT {
		// `show project security` -> `describe app security` (R10's name).
		edit = replaceSpan(verb, stmt.SECURITY().GetSymbol(), keywordLike(verb.GetText(), "describe app security"))
	}
	b.fixLastDeprecation(deprecation.ShowSingleThing, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// showSummaryRemoved is the removal of the one-element summaries: `show
// entity X` and `show association X` (and `list` on them) print a summary no
// mdl 1 statement prints. `describe` prints the definition as MDL and `list
// entities` / `list associations` the summary columns, so neither is an alias,
// and rewriting to either would change the script's output.
var showSummaryRemoved = langver.Change{
	Code:  "MDL-V1-SHOWSUMMARY",
	Since: langver.V1,
	Old:   "`show entity X` / `show association X` prints a summary of the element",
	New: "an error: `show` is dropped (R6) and the summary has no mdl 1 statement; write " +
		"`describe entity X` for the definition, or `list entities in M` / `list associations in M` for the summary",
}

// gateShowSummary gates a one-element summary: refused under mdl 1, kept with
// a warning under mdl 0.
func (b *Builder) gateShowSummary(ctx *parser.ShowOrListContext) {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok {
		return
	}
	kind := "entity"
	if tokenTypeAt(stmt, 1) == parser.MDLParserASSOCIATION {
		kind = "association"
	}
	if b.gate(showSummaryRemoved, stmt) {
		b.addError(fmt.Errorf("line %d: `%s %s` is not in %s: write `describe %s` for the definition, or `list %s` "+
			"for the summary columns",
			stmt.GetStart().GetLine(), strings.ToLower(ctx.GetText()), kind, b.langVersion, kind, pluralKind(kind)))
		return
	}
	b.fixLastNote(showSummaryRemoved.Code, nil, "`"+strings.ToLower(ctx.GetText())+" "+kind+
		"` prints a summary no mdl 1 statement prints: replace it with `describe "+kind+"` (the definition) or `list "+
		pluralKind(kind)+"` (the summary columns)")
}

func pluralKind(kind string) string {
	if kind == "entity" {
		return "entities"
	}
	return kind + "s"
}

// gateShowSession gates `show version|status|connections|catalog status`:
// session state, which a script does not ask for (R7).
func (b *Builder) gateShowSession(ctx *parser.ShowOrListContext) {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok || b.session {
		return // at the REPL or in -c it is where it belongs (BuildSession)
	}
	var words []string
	for i := 1; i < stmt.GetChildCount(); i++ {
		if tn, ok := stmt.GetChild(i).(antlr.TerminalNode); ok {
			words = append(words, strings.ToLower(tn.GetText()))
		}
	}
	cmd := strings.ToLower(ctx.GetText()) + " " + strings.Join(words, " ")
	if b.gate(sessionCommandInScript, stmt) {
		b.addError(fmt.Errorf("line %d: `%s` is a session command, not a model statement: under %s a script "+
			"holds model statements only. Type it at the REPL",
			stmt.GetStart().GetLine(), cmd, b.langVersion))
		return
	}
	if n := len(b.langNotes); n > 0 && b.langNotes[n-1].Code == sessionCommandInScript.Code {
		b.langNotes[n-1].Message = fmt.Sprintf("`%s` is a session command: %s", cmd, b.langNotes[n-1].Message)
	}
}

// recordSingularCollection records `list image|icon|message definition
// collection` for the plural. The lexer's COLLECTION matches both spellings,
// so the token's text says which one was written.
func (b *Builder) recordSingularCollection(ctx *parser.ShowOrListContext) {
	stmt, ok := ctx.GetParent().(*parser.ShowStatementContext)
	if !ok || stmt.COLLECTION() == nil {
		return
	}
	tok := stmt.COLLECTION().GetSymbol()
	if strings.HasSuffix(strings.ToLower(tok.GetText()), "s") {
		return
	}
	b.recordDeprecation(deprecation.SingularCollectionList, tok, "")
	edit := insertAt(tok.GetStop()+1, keywordLike(tok.GetText(), "s"))
	b.fixLastDeprecation(deprecation.SingularCollectionList, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterAlterUserRoleStatement records `remove module roles` for `drop module
// roles`.
func (b *Builder) EnterAlterUserRoleStatement(ctx *parser.AlterUserRoleStatementContext) {
	if ctx.REMOVE() != nil {
		b.recordDeprecation(deprecation.UserRoleRemove, ctx.REMOVE().GetSymbol(), "")
	}
}

// EnterAlterSettingsClause records `remove` on a language or workflow group.
func (b *Builder) EnterAlterSettingsClause(ctx *parser.AlterSettingsClauseContext) {
	if ctx.REMOVE() != nil {
		b.recordDeprecation(deprecation.SettingsRemove, ctx.REMOVE().GetSymbol(), "")
	}
}

// ExitAttributeKw records `column` for `attribute` in alter entity.
func (b *Builder) ExitAttributeKw(ctx *parser.AttributeKwContext) {
	if ctx.COLUMN() != nil {
		b.recordDeprecation(deprecation.ColumnForAttribute, ctx.COLUMN().GetSymbol(), "")
	}
}

// ExitRestCallKw records `rest call` for `call rest service`, rewriting both
// words at once in the letter case of the first.
func (b *Builder) ExitRestCallKw(ctx *parser.RestCallKwContext) {
	if ctx.REST() == nil || ctx.CALL() == nil || ctx.SERVICE() != nil {
		return
	}
	first := ctx.REST().GetSymbol()
	b.recordDeprecation(deprecation.RestCall, first, "")
	edit := replaceSpan(first, ctx.CALL().GetSymbol(), keywordLike(first.GetText(), "call rest service"))
	b.fixLastDeprecation(deprecation.RestCall, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterDescribeStatement records `describe widget <name>` for `describe widget
// type <name>`. Only the widget-definition forms: `describe styling … widget`
// and `describe fragment … widget` name a widget on a page, which is the
// reading `type` exists to keep apart.
func (b *Builder) EnterDescribeStatement(ctx *parser.DescribeStatementContext) {
	if ctx.WIDGET() == nil || ctx.TYPE() != nil || ctx.STYLING() != nil || ctx.FRAGMENT() != nil {
		return
	}
	w := ctx.WIDGET().GetSymbol()
	b.recordDeprecation(deprecation.DescribeWidgetType, w, "")
	edit := insertAt(w.GetStop()+1, " "+keywordLike(w.GetText(), "type"))
	b.fixLastDeprecation(deprecation.DescribeWidgetType, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

// EnterDefineFragmentStatement records `define fragment` for `create fragment`.
func (b *Builder) EnterDefineFragmentStatement(ctx *parser.DefineFragmentStatementContext) {
	if ctx.DEFINE() != nil {
		b.recordDeprecation(deprecation.DefineFragment, ctx.DEFINE().GetSymbol(), "")
	}
}
