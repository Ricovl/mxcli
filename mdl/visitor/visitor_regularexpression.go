// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateRegularExpressionStatement builds a CreateRegularExpressionStmt from
// CREATE [OR REPLACE|MODIFY] REGULAR EXPRESSION Module.Name ( ... ).
func (b *Builder) ExitCreateRegularExpressionStatement(ctx *parser.CreateRegularExpressionStatementContext) {
	stmt := &ast.CreateRegularExpressionStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	if body := ctx.RegularExpressionBody(); body != nil {
		bodyCtx := body.(*parser.RegularExpressionBodyContext)
		for i, prop := range bodyCtx.AllRegularExpressionProperty() {
			pc, ok := prop.(*parser.RegularExpressionPropertyContext)
			if !ok || pc == nil {
				continue
			}
			iok := pc.IdentifierOrKeyword(0)
			if iok == nil {
				continue
			}
			switch strings.ToLower(identifierOrKeywordText(iok)) {
			case "expression", "pattern":
				stmt.Expression = regularExpressionPropertyText(pc)
			case "exportlevel":
				stmt.ExportLevel = regularExpressionPropertyText(pc)
				if strings.EqualFold(stmt.ExportLevel, "Public") {
					stmt.ExportLevel = "API"
					b.recordRegexExportLevelPublic(pc, stmt.Name.String())
				}
			case "documentation":
				// R9: an alias of the doc comment, which it overrides.
				stmt.Documentation, stmt.DocumentationSet = regularExpressionPropertyText(pc), true
				b.recordDocumentationProperty(ctx, ruleContexts(bodyCtx.AllRegularExpressionProperty()), i, stmt.Documentation)
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// regularExpressionPropertyText returns the value side of a property, unquoted.
//
// The key is identifierOrKeyword(0), so an identifier value is index 1 —
// reading index 0 would echo the key back as the value.
func regularExpressionPropertyText(pc *parser.RegularExpressionPropertyContext) string {
	if s := pc.STRING_LITERAL(); s != nil {
		return unquoteStringLit(s)
	}
	if bl := pc.BooleanLiteral(); bl != nil {
		return bl.GetText()
	}
	if v := pc.IdentifierOrKeyword(1); v != nil {
		return identifierOrKeywordText(v)
	}
	return ""
}

// recordRegexExportLevelPublic records `ExportLevel: Public` (MDL-DEPR161,
// ako/mxcli#827). The metamodel's export levels are Hidden and API; Public was
// accepted and written verbatim, a value Studio Pro does not have. It is a
// respelling of API and builds API; fmt --upgrade writes API.
func (b *Builder) recordRegexExportLevelPublic(pc *parser.RegularExpressionPropertyContext, subject string) {
	var value antlr.ParserRuleContext
	repl := "API"
	if s := pc.STRING_LITERAL(); s != nil {
		tok := s.GetSymbol()
		b.recordDeprecation(deprecation.RegexExportLevelPublic, tok, subject)
		b.fixLastDeprecation(deprecation.RegexExportLevelPublic, &ast.Fix{Edits: []ast.TextEdit{
			{Start: tok.GetStart(), Stop: tok.GetStop() + 1, Text: "'" + repl + "'"},
		}}, "")
		return
	}
	if v := pc.IdentifierOrKeyword(1); v != nil {
		value, _ = v.(antlr.ParserRuleContext)
	}
	if value == nil || value.GetStart() == nil || value.GetStop() == nil {
		return
	}
	b.recordDeprecation(deprecation.RegexExportLevelPublic, value.GetStart(), subject)
	b.fixLastDeprecation(deprecation.RegexExportLevelPublic, &ast.Fix{Edits: []ast.TextEdit{
		{Start: value.GetStart().GetStart(), Stop: value.GetStop().GetStop() + 1, Text: repl},
	}}, "")
}
