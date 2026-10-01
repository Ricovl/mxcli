// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// A name on a page element Mendix stores no name for (R12, ako/mxcli#749).
//
// A layout grid's rows and columns, a data grid's columns and control bar, a
// gallery's template and filter, and a data view's footer have no Name in the
// model: the builder of
// the parent builds them and never reads one. describe used to invent one
// (`row1`, `col3`, a column named after its attribute) and scripts copied it,
// so a name there is the old spelling of the same page — MDL-DEPR005. It is
// dropped from the AST, which makes the two spellings build identical
// statements, the registry's proof that the rewrite changes nothing.
//
// Whether a name is stored depends on the PARENT: a `row` directly in a
// `layoutgrid` has none, while a `row` anywhere else is built as a container
// that keeps it (its columns, though, are layout grid columns either way). So
// the decision is made when the parent is built, from the spans
// rememberWidgetName kept for its children.

// widgetNameSpan is where a widget's name was written: the last token of its
// type keyword and the name token itself.
type widgetNameSpan struct {
	typeStop antlr.Token
	name     antlr.Token
}

// unstoredChildKinds maps a parent widget type to the child kinds it stores
// without a name. Only the built-in keywords: a def-driven widget's own item
// and slot keywords need the widget registry, which the visitor has not got.
var unstoredChildKinds = map[string]map[string]bool{
	"layoutgrid": {"row": true},
	// A row's columns are layout grid columns wherever the row is: a
	// standalone row is built as a container holding a one-row layout grid,
	// and the container — not the columns — keeps the row's name.
	"row":      {"column": true},
	"datagrid": {"column": true, "controlbar": true},
	"gallery":  {"template": true, "filter": true},
	// A data view's footer is a region: its widgets go to the data view's
	// FooterWidgets and the footer itself is not stored, so its name could
	// never be addressed (ako/mxcli#528). `dvMain.footer` addresses it.
	"dataview": {"footer": true},
}

// rememberWidgetName records where a widget's name was written, for its parent.
func (b *Builder) rememberWidgetName(w *ast.WidgetV3, ctx *parser.WidgetV3Context, name antlr.Token) {
	typeCtx := ctx.WidgetTypeV3()
	if typeCtx == nil || name == nil {
		return
	}
	if b.widgetNameSpans == nil {
		b.widgetNameSpans = map[*ast.WidgetV3]widgetNameSpan{}
	}
	b.widgetNameSpans[w] = widgetNameSpan{typeStop: typeCtx.GetStop(), name: name}
}

// dropUnstoredChildNames reports and drops the name of every child of w that
// the model stores without one.
func (b *Builder) dropUnstoredChildNames(w *ast.WidgetV3) {
	kinds := unstoredChildKinds[strings.ToLower(w.Type)]
	if kinds == nil {
		return
	}
	for _, child := range w.Children {
		kind := strings.ToLower(child.Type)
		if !kinds[kind] || child.Specialization != "" {
			continue
		}
		b.dropUnstoredName(child, kind)
	}
}

func (b *Builder) dropUnstoredName(w *ast.WidgetV3, kind string) {
	span, ok := b.widgetNameSpans[w]
	if !ok || w.Name == "" {
		return
	}
	w.Name = ""
	b.recordDeprecation(deprecation.UnstoredWidgetName, span.name, kind)
	// Delete the name and the space before it: `row row1 {` -> `row {`.
	edit := replaceGap(span.typeStop.GetStop(), span.name.GetStop()+1, "")
	b.fixLastDeprecation(deprecation.UnstoredWidgetName, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}
