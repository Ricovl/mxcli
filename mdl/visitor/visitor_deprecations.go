// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"

	"github.com/antlr4-go/antlr/v4"
)

// Deprecated spellings are recorded from the parse tree because it is the only
// place they are visible: `create or replace` and `create or modify`, `show`
// and `list`, build the same statements. The registry of spellings, and how a
// grammar alias is marked, is in mdl/deprecation.

// recordDeprecation appends one use of a deprecated spelling at tok.
func (b *Builder) recordDeprecation(code string, tok antlr.Token, subject string) {
	if tok == nil {
		return
	}
	b.deprecations = append(b.deprecations, ast.DeprecatedSpelling{
		Code:    code,
		Line:    tok.GetLine(),
		Column:  tok.GetColumn(),
		Subject: subject,
	})
}

// createOrReplaceIsNotAnAlias lists the create kinds (createStatementKind's
// words) where `or replace` does NOT mean `or modify`, so rewriting it would
// change the script. Measured against the visitors, and pinned by
// TestCreateOrReplaceMatchesModifyExceptExemptKinds:
//
//   - translations: `or replace` replaces the whole set, `or modify` merges.
//   - userrole, demouser: the visitor reads only `or modify`; `or replace` is
//     a plain create, which fails on an existing role or user.
//
// `create or replace view entity` (kind "entity") drops and recreates the view
// entity, and is exempted in recordCreateOrReplace.
var createOrReplaceIsNotAnAlias = map[string]bool{
	"translations": true,
	"userrole":     true,
	"demouser":     true,
}

// recordCreateOrReplace records MDL-DEPR001 for a `create or replace` whose
// meaning is exactly `create or modify`.
func (b *Builder) recordCreateOrReplace(ctx *parser.CreateStatementContext) {
	if ctx.OR() == nil || ctx.REPLACE() == nil {
		return
	}
	kind := createStatementKind(ctx)
	if kind == "" || createOrReplaceIsNotAnAlias[kind] {
		return
	}
	if ent, ok := ctx.CreateEntityStatement().(*parser.CreateEntityStatementContext); ok && ent.VIEW() != nil {
		return
	}
	b.recordDeprecation(deprecation.CreateOrReplace, ctx.REPLACE().GetSymbol(), kind)
}

// ExitShowOrList records MDL-DEPR002 for `show`. showOrList is used only by
// showStatement, where `show` and `list` build the same statement.
func (b *Builder) ExitShowOrList(ctx *parser.ShowOrListContext) {
	if ctx == nil || ctx.SHOW() == nil {
		return
	}
	b.recordDeprecation(deprecation.Show, ctx.SHOW().GetSymbol(), "")
}
