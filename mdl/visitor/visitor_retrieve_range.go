// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// limitOneIsAList is the change of meaning of `retrieve … limit 1` (ako/mxcli#734,
// PROPOSAL_mdl_beta_syntax_freeze.md §5 item 2).
//
// In the alpha language `limit 1` without an offset binds ONE object: the writer
// stores Mendix's "First object" range. Every SQL reader expects a list of one,
// and `import from mapping` already spells the object `first` and a list of one
// `limit 1`. Under mdl 1 `limit 1` is the Custom range, a list; the object is
// spelled `first` in both versions.
var limitOneIsAList = langver.Change{
	Code:  "MDL-V1-LIMIT1",
	Since: langver.V1,
	Old: "`retrieve … limit 1` binds a single object (Mendix's \"First object\" range, " +
		"so a loop over it is CE0100 and count() or head() CE0097); write `retrieve … first` to say so",
	New: "a list of one (a Custom range), and the object is written `first`",
}

// ExitRetrieveStatement warns on `limit 1` kept at its alpha meaning. The
// statement builder, a free function, applies the same decision through
// retrieveLimitOneIsObject; this is where the note is recorded, once.
//
// It also refuses `first` on an association retrieve: Mendix gives that source
// no range, so the keyword would be dropped on the way to the model, and
// whether the result is an object or a list follows from the association.
func (b *Builder) ExitRetrieveStatement(ctx *parser.RetrieveStatementContext) {
	b.refuseRetrieveStringXPathValues(ctx)
	if ctx.FIRST() != nil && isAssociationRetrieve(ctx) {
		b.addError(fmt.Errorf("line %d: `first` does not apply to a retrieve over an association: Mendix gives "+
			"that source no range, and whether it binds an object or a list follows from the association. "+
			"Drop `first`, or retrieve from the database (`retrieve $x from Module.Entity where … first`)",
			ctx.GetStart().GetLine()))
		return
	}
	if isBareLimitOne(ctx) && !b.gate(limitOneIsAList, ctx) {
		// `limit 1` -> `first`: the object range, spelled the way that means
		// it under every version.
		limit := ctx.LIMIT().GetSymbol()
		b.fixLastNote(limitOneIsAList.Code, &ast.Fix{Edits: []ast.TextEdit{
			replaceSpan(limit, ctx.GetLimitExpr().GetStop(), keywordLike(limit.GetText(), "first")),
		}}, "")
	}
}

// isAssociationRetrieve reports whether the retrieve's source is `$Var/Assoc`.
func isAssociationRetrieve(ctx *parser.RetrieveStatementContext) bool {
	src, ok := ctx.RetrieveSource().(*parser.RetrieveSourceContext)
	return ok && src != nil && src.VARIABLE() != nil
}

// retrieveLimitOneIsObject reports whether ctx is a `limit 1` that binds an
// object: a bare `limit 1` in a script written before mdl 1.
func retrieveLimitOneIsObject(ctx *parser.RetrieveStatementContext) bool {
	return isBareLimitOne(ctx) && !limitOneIsAList.Applies(languageVersionOf(ctx))
}

// isBareLimitOne reports whether a retrieve's range is exactly `limit 1` with no
// offset, on a source that is not an association: the one spelling whose
// meaning depends on the language version.
//
// The limit is compared as the builder stores it (source text plus the
// whitespace it keeps before `;`), which is the condition the alpha writer has
// always used to choose the object range: `limit 1` is an object, `limit (1)`
// or `limit $One` a list.
func isBareLimitOne(ctx *parser.RetrieveStatementContext) bool {
	if ctx == nil || ctx.FIRST() != nil || ctx.GetOffsetExpr() != nil || ctx.GetLimitExpr() == nil {
		return false
	}
	if isAssociationRetrieve(ctx) {
		return false // an association retrieve has no range
	}
	limit := ctx.GetLimitExpr()
	return retrieveRangeExpressionSource(limit)+retrieveLimitTrailingWhitespace(ctx, limit) == "1"
}

// languageVersionOf reads the `mdl <n>;` header of the script tree is part of.
// The header can only be the first statement, so it is on the root. It exists
// for the statement builders, which are free functions without the Builder and
// must still build the AST of the version the script is written in. A missing
// or malformed header is mdl 0 (ExitLanguageHeader reports a malformed one).
func languageVersionOf(tree antlr.Tree) langver.Version {
	for ; tree != nil; tree = tree.GetParent() {
		prog, ok := tree.(*parser.ProgramContext)
		if !ok {
			continue
		}
		h, ok := prog.LanguageHeader().(*parser.LanguageHeaderContext)
		if !ok || h == nil || h.IDENTIFIER() == nil || h.NUMBER_LITERAL() == nil ||
			!strings.EqualFold(h.IDENTIFIER().GetText(), "mdl") {
			return langver.V0
		}
		n, err := strconv.Atoi(h.NUMBER_LITERAL().GetText())
		if v := langver.Version(n); err == nil && v.Known() {
			return v
		}
		return langver.V0
	}
	return langver.V0
}
