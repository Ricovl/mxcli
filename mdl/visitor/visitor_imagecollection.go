// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateImageCollectionStatement is called when exiting the createImageCollectionStatement production.
func (b *Builder) ExitCreateImageCollectionStatement(ctx *parser.CreateImageCollectionStatementContext) {
	stmt := &ast.CreateImageCollectionStmt{
		Name:        buildQualifiedName(ctx.QualifiedName()),
		ExportLevel: "Hidden",
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteStringLit(lit)
	}

	// Extract /** ... */ doc comment (same as other create statements)
	stmt.Comment, stmt.DocumentationSet = findDocComment(ctx)

	if opts := ctx.ImageCollectionOptions(); opts != nil {
		optsCtx := opts.(*parser.ImageCollectionOptionsContext)
		for _, opt := range optsCtx.AllImageCollectionOption() {
			optCtx := opt.(*parser.ImageCollectionOptionContext)
			if optCtx.EXPORT() != nil && optCtx.LEVEL() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.ExportLevel = unquoteStringLit(optCtx.STRING_LITERAL())
			}
			if c := optCtx.COMMENT(); c != nil && optCtx.STRING_LITERAL() != nil {
				// R9: `comment '…'` is the documentation, which a doc
				// comment also states; the clause wins, as it always has.
				text := unquoteStringLit(optCtx.STRING_LITERAL())
				b.recordDocumentationClause(ctx, c.GetSymbol(), optCtx.STRING_LITERAL().GetSymbol(), text, true)
				stmt.Comment, stmt.DocumentationSet = text, true
			}
		}
	}

	if body := ctx.ImageCollectionBody(); body != nil {
		bodyCtx := body.(*parser.ImageCollectionBodyContext)
		// { image Name ( File: '…' ) } — the canonical form.
		for _, child := range bodyCtx.AllImageCollectionChild() {
			childCtx := child.(*parser.ImageCollectionChildContext)
			name := imageNameText(childCtx.ImageName())
			item := ast.ImageItem{Name: name}
			hasFile := false
			for _, p := range childCtx.AllImageProperty() {
				pc := p.(*parser.ImagePropertyContext)
				key := identifierOrKeywordText(pc.IdentifierOrKeyword())
				if !strings.EqualFold(key, "File") {
					b.addError(fmt.Errorf("line %d: unknown property '%s' on image %s: an image takes only File: '<path>'",
						pc.GetStart().GetLine(), key, name))
					continue
				}
				item.FilePath = unquoteStringLit(pc.STRING_LITERAL())
				hasFile = true
			}
			if !hasFile {
				b.addError(fmt.Errorf("line %d: image %s has no File: an image is `image %s ( File: '<path>' )`",
					childCtx.GetStart().GetLine(), name, name))
			}
			stmt.Images = append(stmt.Images, item)
		}
		// ( image Name from file '…', … ) — the old spelling (MDL-DEPR072).
		for _, item := range bodyCtx.AllImageCollectionItem() {
			itemCtx := item.(*parser.ImageCollectionItemContext)
			stmt.Images = append(stmt.Images, ast.ImageItem{
				Name:     imageNameText(itemCtx.ImageName()),
				FilePath: unquoteStringLit(itemCtx.GetPath()),
			})
		}
	}

	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil && createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
		stmt.CreateOrModify = true
	}

	b.statements = append(b.statements, stmt)
}

// imageNameText is an image's name, without the quotes of a quoted identifier
// ("Name" or `Name`).
func imageNameText(ctx parser.IImageNameContext) string {
	name := ctx.GetText()
	if len(name) >= 2 && (name[0] == '"' || name[0] == '`') {
		name = name[1 : len(name)-1]
	}
	return name
}
