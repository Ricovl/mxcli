// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R5 (ADR-0010; ako/mxcli#753): a constant is referred to one way everywhere,
// `@Module.Const` — the spelling a Mendix expression uses. The other spellings
// are deprecated aliases whose use is recorded here with the rewrite to `@`.

// dollarConstantName is the qualified name a consumed REST service's `$Const`
// credential names: a constant of the service's own module, unless the name is
// already qualified.
func dollarConstantName(module, variable string) string {
	name := strings.TrimPrefix(variable, "$")
	if strings.Contains(name, ".") || module == "" {
		return name
	}
	return module + "." + name
}

// recordDollarConstant records MDL-DEPR083 on a `$Const` credential, with the
// rewrite to `@Module.Const`.
func (b *Builder) recordDollarConstant(v antlr.TerminalNode, module string) {
	tok := v.GetSymbol()
	b.recordDeprecation(deprecation.DollarConstant, tok, "constant reference")
	name := dollarConstantName(module, tok.GetText())
	if !strings.Contains(name, ".") {
		b.fixLastDeprecation(deprecation.DollarConstant, nil, "the service has no module to qualify "+tok.GetText()+" with")
		return
	}
	b.fixLastDeprecation(deprecation.DollarConstant,
		&ast.Fix{Edits: []ast.TextEdit{replaceSpan(tok, tok, "@"+name)}}, "")
}

// ExitModelProperty records an agent-editor `Key: Module.Const` written
// without its `@`.
func (b *Builder) ExitModelProperty(ctx *parser.ModelPropertyContext) {
	idents := ctx.AllIdentifierOrKeyword()
	if len(idents) == 0 || !strings.EqualFold(idents[0].GetText(), "key") || ctx.AT() != nil {
		return
	}
	b.recordBareConstantKey(ctx.QualifiedName())
}

// ExitAgentEditorAlterAssignment records `set Key = Module.Const` written
// without its `@`.
func (b *Builder) ExitAgentEditorAlterAssignment(ctx *parser.AgentEditorAlterAssignmentContext) {
	iok := ctx.IdentifierOrKeyword()
	val, ok := ctx.AgentEditorAlterValue().(*parser.AgentEditorAlterValueContext)
	if iok == nil || !ok || val == nil || !strings.EqualFold(iok.GetText(), "key") || val.AT() != nil {
		return
	}
	b.recordBareConstantKey(val.QualifiedName())
}

// recordBareConstantKey records MDL-DEPR084 on a constant named without `@`,
// with the rewrite that adds it.
func (b *Builder) recordBareConstantKey(qn parser.IQualifiedNameContext) {
	if qn == nil || qn.GetStart() == nil {
		return
	}
	b.recordDeprecation(deprecation.BareConstantKey, qn.GetStart(), "constant reference")
	b.fixLastDeprecation(deprecation.BareConstantKey,
		&ast.Fix{Edits: []ast.TextEdit{insertAt(qn.GetStart().GetStart(), "@")}}, "")
}

// settingsConstantRefText is the qualified name a settings constant
// reference names: `@M.C` or, deprecated, `'M.C'`.
func settingsConstantRefText(ctx parser.ISettingsConstantRefContext) string {
	if ctx == nil {
		return ""
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		return unquoteStringLit(lit)
	}
	return ""
}

// ExitSettingsConstantRef records MDL-DEPR085 on a constant named in a string,
// with the rewrite to `@Module.Const` when the string holds a qualified name
// that reads back as itself.
func (b *Builder) ExitSettingsConstantRef(ctx *parser.SettingsConstantRefContext) {
	lit := ctx.STRING_LITERAL()
	if lit == nil {
		return
	}
	b.recordDeprecation(deprecation.QuotedSettingsConstant, lit.GetSymbol(), "constant reference")
	name := unquoteStringLit(lit)
	qn, ok := parseRule(name, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.QualifiedName() })
	if holdsInterpretedEscape(lit) || !ok || getQualifiedNameText(qn.(parser.IQualifiedNameContext)) != name || !strings.Contains(name, ".") {
		b.fixLastDeprecation(deprecation.QuotedSettingsConstant, nil,
			"the string "+lit.GetText()+" is not a Module.Constant name; write it as @Module.Constant by hand")
		return
	}
	b.fixLastDeprecation(deprecation.QuotedSettingsConstant,
		&ast.Fix{Edits: []ast.TextEdit{replaceSpan(lit.GetSymbol(), lit.GetSymbol(), "@"+name)}}, "")
}
