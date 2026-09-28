// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"encoding/base64"
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
				key := identifierOrKeywordText(pc.IdentifierOrKeyword(0))
				line := pc.GetStart().GetLine()
				// A string value, or a bare word (the Format's only shape).
				str, isStr := "", pc.STRING_LITERAL() != nil
				if isStr {
					str = unquoteStringLit(pc.STRING_LITERAL())
				} else if w := pc.IdentifierOrKeyword(1); w != nil {
					str = identifierOrKeywordText(w)
				}
				switch strings.ToLower(key) {
				case "file":
					if !isStr {
						b.addError(fmt.Errorf("line %d: image %s: File takes a path in a string: File: '<path>'", line, name))
						continue
					}
					item.FilePath = str
					hasFile = true
				case "data":
					// The image itself, base64-encoded: what describe writes, so
					// its output does not name a file it wrote (ako/mxcli#707).
					data, err := base64.StdEncoding.DecodeString(str)
					if !isStr || err != nil {
						b.addError(fmt.Errorf("line %d: image %s: Data takes the image base64-encoded in a string", line, name))
						continue
					}
					item.Data, item.HasData = data, true
					hasFile = true
				case "format":
					f, ok := imageFormatName(str)
					if !ok {
						b.addError(fmt.Errorf("line %d: image %s: unknown Format '%s'; an image format is png, jpg, gif, svg, bmp or webp",
							line, name, str))
						continue
					}
					item.Format = f
				default:
					b.addError(fmt.Errorf("line %d: unknown property '%s' on image %s: an image takes File: '<path>', "+
						"or Data: '<base64>' with an optional Format", line, key, name))
				}
			}
			if item.FilePath != "" && item.HasData {
				b.addError(fmt.Errorf("line %d: image %s has both File and Data; it takes one of them",
					childCtx.GetStart().GetLine(), name))
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

// imageFormatName maps a written image format to the Mendix ImageFormat
// value, in any letter case; `jpeg` is `jpg`.
func imageFormatName(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "png":
		return "Png", true
	case "jpg", "jpeg":
		return "Jpg", true
	case "gif":
		return "Gif", true
	case "svg":
		return "Svg", true
	case "bmp":
		return "Bmp", true
	case "webp":
		return "Webp", true
	}
	return "", false
}
