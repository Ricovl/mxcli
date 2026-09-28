// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R10 (PROPOSAL_mdl_beta_syntax_freeze.md §3; ADR-0010): a document type is
// named as Studio Pro names it. Each name is spelt by one grammar rule (the
// *Kw rules in MDLService.g4 and MDLSecurity.g4), whose canonical alternative
// comes first and whose old mxcli spelling is a registered alias. Both build
// the same statement, so the parse tree is the only place the old spelling is
// still visible: the listeners below record it, with the rewrite that replaces
// the whole name (`rest client` -> `consumed rest service`) in the letter case
// of its first word.

// recordDocumentName records code for a deprecated document-type name spanning
// ctx, when the rule matched its alias alternative (old reports that).
func (b *Builder) recordDocumentName(code string, ctx antlr.ParserRuleContext, old bool, canonical string) {
	if ctx == nil || !old || ctx.GetStart() == nil || ctx.GetStop() == nil {
		return
	}
	first := ctx.GetStart()
	b.recordDeprecation(code, first, canonical)
	edit := replaceSpan(first, ctx.GetStop(), keywordLike(first.GetText(), canonical))
	b.fixLastDeprecation(code, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
}

func (b *Builder) ExitConsumedRestServiceKw(ctx *parser.ConsumedRestServiceKwContext) {
	b.recordDocumentName(deprecation.ConsumedRestService, ctx, ctx.CLIENT() != nil, "consumed rest service")
}

func (b *Builder) ExitConsumedRestServicesKw(ctx *parser.ConsumedRestServicesKwContext) {
	b.recordDocumentName(deprecation.ConsumedRestService, ctx, ctx.CLIENTS() != nil, "consumed rest services")
}

func (b *Builder) ExitConsumedODataServiceKw(ctx *parser.ConsumedODataServiceKwContext) {
	b.recordDocumentName(deprecation.ConsumedODataService, ctx, ctx.CLIENT() != nil, "consumed odata service")
}

func (b *Builder) ExitConsumedODataServicesKw(ctx *parser.ConsumedODataServicesKwContext) {
	b.recordDocumentName(deprecation.ConsumedODataService, ctx, ctx.CLIENTS() != nil, "consumed odata services")
}

func (b *Builder) ExitPublishedODataServiceKw(ctx *parser.PublishedODataServiceKwContext) {
	b.recordDocumentName(deprecation.PublishedODataService, ctx, ctx.PUBLISHED() == nil, "published odata service")
}

func (b *Builder) ExitPublishedODataServicesKw(ctx *parser.PublishedODataServicesKwContext) {
	b.recordDocumentName(deprecation.PublishedODataService, ctx, ctx.PUBLISHED() == nil, "published odata services")
}

func (b *Builder) ExitTaskQueueKw(ctx *parser.TaskQueueKwContext) {
	b.recordDocumentName(deprecation.TaskQueue, ctx, ctx.TASK() == nil, "task queue")
}

func (b *Builder) ExitTaskQueuesKw(ctx *parser.TaskQueuesKwContext) {
	b.recordDocumentName(deprecation.TaskQueue, ctx, ctx.TASK() == nil, "task queues")
}

func (b *Builder) ExitAppSecurityKw(ctx *parser.AppSecurityKwContext) {
	b.recordDocumentName(deprecation.AppSecurity, ctx, ctx.PROJECT() != nil, "app security")
}

func (b *Builder) ExitAiModelKw(ctx *parser.AiModelKwContext) {
	b.recordDocumentName(deprecation.AIModel, ctx, ctx.AI() == nil, "ai model")
}

func (b *Builder) ExitAiModelsKw(ctx *parser.AiModelsKwContext) {
	b.recordDocumentName(deprecation.AIModel, ctx, ctx.AI() == nil, "ai models")
}

func (b *Builder) ExitJsonSampleKw(ctx *parser.JsonSampleKwContext) {
	b.recordDocumentName(deprecation.JSONStructureSample, ctx, ctx.SNIPPET() != nil, "sample")
}

func (b *Builder) ExitSettingsSection(ctx *parser.SettingsSectionContext) {
	b.recordDocumentName(deprecation.SettingsRuntime, ctx, ctx.MODEL() != nil, "runtime")
}

// settingsSectionName is the section an `alter settings` clause names, as the
// executor matches it (case-insensitively). `runtime`, Studio Pro's name for
// the tab that holds Settings$ModelSettings, is the section mxcli has always
// called `model`; both build the same statement.
func settingsSectionName(ctx parser.ISettingsSectionContext) string {
	// Lowercase, like every settings section in the AST (R8): the executor
	// matches the section on its lowercase name.
	if sc, ok := ctx.(*parser.SettingsSectionContext); ok && sc.RUNTIME() != nil {
		return "model"
	}
	return strings.ToLower(ctx.GetText())
}
