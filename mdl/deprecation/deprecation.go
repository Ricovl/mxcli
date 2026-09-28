// SPDX-License-Identifier: Apache-2.0

// Package deprecation is the single registry of deprecated MDL spellings
// (ADR-0011, decision 1).
//
// A deprecated spelling is a respelling: it means exactly what its canonical
// form means, so it keeps parsing, `check` and `exec` warn on it, and
// `mxcli fmt --upgrade` can rewrite it mechanically. A spelling that means
// something different from the proposed canonical form is NOT an alias there,
// and is not reported — rewriting it would silently change a script.
//
// # How an alias is marked
//
// Every grammar token or alternative that exists only as an alias carries a
// block comment naming its registry code, next to the alias itself:
//
//	CREATE (OR (MODIFY | REPLACE /* @alias MDL-DEPR001 */))?
//
// ANTLR ignores the comment. TestGrammarAliasesAreRegistered (mdl/grammar)
// reads the .g4 sources and fails when a marker names a code with no entry
// here, or an entry is named by no marker. The marker is what makes a missing
// entry detectable: an alias is marked in the edit that adds it, and the marker
// cannot be satisfied without an entry.
//
// # How a use is detected
//
// Both spellings build the same AST, so the parse tree is the only place the
// source spelling is still visible. The visitor records every use of a
// registered spelling on ast.Program.Deprecations, and the executor's
// ValidateDeprecations turns the records into warnings (errors under
// --deprecations=error). This generalises MDL065, where the AST node itself
// carries the spelling flags.
package deprecation

import (
	"fmt"
	"strings"
)

// Entry is one deprecated spelling.
type Entry struct {
	// Code is the stable warning code, MDL-DEPRnnn. Never reused.
	Code string
	// Old is the deprecated form, as a reader would write it.
	Old string
	// Canonical is the form that replaces it (ADR-0010).
	Canonical string
	// Rewrite is the mechanical rewrite `fmt --upgrade` applies.
	Rewrite Rewrite
	// RemovedIn is the MDL language version (the `mdl <n>;` header) under which
	// the old form is refused. Under earlier versions it warns (ADR-0011).
	RemovedIn int
	// Note is shown with the warning: scope limits, or where the canonical form
	// is expected to move next.
	Note string
	// Example is a complete statement in the old form. A test parses it and
	// requires exactly this code to be recorded.
	Example string
	// CanonicalExample is Example after Rewrite. A test requires it to record no
	// deprecation and to build the same AST as Example — the proof that the
	// rewrite does not change meaning.
	CanonicalExample string
}

// Rewrite is the mechanical rewrite from the deprecated form to the canonical
// one. It is either a keyword swap (Token and Replacement) or, where the two
// forms differ in shape rather than in one word, a structural rewrite that
// Structural names. A structural rewrite is implemented against the parse tree,
// never as text substitution, and its correctness rests on the same test as a
// swap: Example and CanonicalExample must build the same statements.
type Rewrite struct {
	// Token is the keyword to replace, lower-case.
	Token string
	// Replacement is the keyword written in its place, lower-case.
	Replacement string
	// Structural describes a rewrite that is not a keyword swap, e.g.
	// "call form to statement form". Empty for a keyword swap.
	Structural string
}

// IsZero reports whether r is empty: the entry has no mechanical rewrite, and
// `fmt --upgrade` reports its uses instead of guessing at one. A structural
// rewrite is computed per use by the visitor that records it (ast.Fix), and a
// use it cannot rewrite is reported the same way.
func (r Rewrite) IsZero() bool { return r.Token == "" && r.Replacement == "" && r.Structural == "" }

// Codes of the registered entries, for the visitor to record.
const (
	CreateOrReplace = "MDL-DEPR001"
	Show            = "MDL-DEPR002"
	// ListOperationFunctionForm is `$x = head($L)` and the other list
	// operations written as calls; find and contains are excluded, because the
	// call form clashes with the string functions (see mdl/visitor, MDL-V1-LIST).
	ListOperationFunctionForm = "MDL-DEPR003"
	// AggregateFunctionForm is `$n = count($L)` and the other aggregates
	// written as calls.
	AggregateFunctionForm = "MDL-DEPR004"
	// UnstoredWidgetName is a name written on a page element Mendix stores no
	// name for: a layout grid's rows and columns, a data grid's columns and
	// control bar, a gallery's template and filter (R12, ako/mxcli#749).
	UnstoredWidgetName = "MDL-DEPR005"

	// R10: document types named as Studio Pro names them (ako/mxcli#755). The
	// codes are a block of their own so parallel work does not collide.
	ConsumedRestService   = "MDL-DEPR550" // rest client -> consumed rest service
	ConsumedODataService  = "MDL-DEPR551" // odata client -> consumed odata service
	PublishedODataService = "MDL-DEPR552" // odata service -> published odata service
	TaskQueue             = "MDL-DEPR553" // queue -> task queue
	AppSecurity           = "MDL-DEPR554" // project security -> app security
	SettingsRuntime       = "MDL-DEPR555" // alter settings model -> alter settings runtime
	// ReversedEntityGrant is `grant M.Role on M.E (rights) where '…'`, the only
	// grant with the role first and the XPath in a string (R5, ako/mxcli#753).
	// Codes 006-029 are taken by the other wave-2 changes in flight.
	ReversedEntityGrant = "MDL-DEPR030"
	// QuotedTargetingXPath is a workflow user task's `targeting xpath '…'`,
	// the XPath in a string instead of in [ ] (R5, ako/mxcli#753).
	QuotedTargetingXPath = "MDL-DEPR031"

	// Codes 020–029 are R8's (ako/mxcli#752, PROPOSAL_mdl_beta_syntax_freeze.md
	// §3 R8): words, not SCREAMING_SNAKE, and one spelling per keyword. They
	// start at 020 rather than 006 because the other phase-3 issues add entries
	// in parallel; a gap in the numbering means nothing.

	// PageActionWord is a page action written as one snake-case token —
	// `show_page`, `save_changes`, `close_page`, `create_object`,
	// `delete_object`, `open_link`, `sign_out`, `complete_task`,
	// `cancel_changes` — or a flow call without `call` (`microflow M.F`).
	PageActionWord = "MDL-DEPR020"
	// ErrorMessageKeyword is the user-facing message of a validation, spelled
	// anything but `error message`: `not null error '…'` (also after unique
	// and required), a validation rule's `feedback '…'`, and `error_message` /
	// `errormessage`.
	ErrorMessageKeyword = "MDL-DEPR021"
	// DeleteBehaviorClause is an association's `delete_behavior <behaviour>`
	// clause, in any of its spellings; `on delete …` says the same thing.
	DeleteBehaviorClause = "MDL-DEPR022"
	// ReferenceSetUnderscore is `reference_set` for the `ReferenceSet` type.
	ReferenceSetUnderscore = "MDL-DEPR023"
	// ReturnsNone is a REST call's `returns none`, the second spelling of
	// `returns nothing`.
	ReturnsNone = "MDL-DEPR024"
	// OnErrorBraces is a custom error handler written `on error { … }`: the
	// only brace block inside a microflow, where flow is `begin … end <keyword>`
	// (R2, ako/mxcli#754).
	OnErrorBraces = "MDL-DEPR540"
	// DollarArgumentName is `$Param = expr` at a call site: the parameter
	// named with the `$` of a variable (R4, ako/mxcli#751).
	DollarArgumentName = "MDL-DEPR006"
	// ColonArgument is `Param: expr` at a call site (`show page`, a page
	// action or data source): `:` sets a model property, `=` binds a value
	// (R3/R4, ako/mxcli#751).
	ColonArgument = "MDL-DEPR007"
	// WorkflowStringArgument is a workflow call's `with (Param = '<expr>')`:
	// the argument expression written inside a string (R4, ako/mxcli#751).
	WorkflowStringArgument = "MDL-DEPR008"
	// PositionalTemplateArguments is `objects [a, b]` / `parameters [a, b]`
	// on a text template: the placeholders bound by position (R4,
	// ako/mxcli#751).
	PositionalTemplateArguments = "MDL-DEPR009"

	// R9 (ako/mxcli#755): where document metadata lives. Codes 100–119 are
	// this change's block; 101–103 are left to the parallel work that took them.

	// DocumentationClause is the `comment '…'` clause on a constant,
	// association, JSON structure or image collection: the document's
	// documentation, which every other document takes as a `/** … */` doc
	// comment.
	DocumentationClause = "MDL-DEPR100"
	// WorkflowCommentCaption is a workflow activity's `comment '…'`, which sets
	// the activity's caption, not a comment.
	WorkflowCommentCaption = "MDL-DEPR104"
	// FolderProperty is `Folder: '…'` in a page, snippet, consumed REST
	// service, consumed or published OData service or published REST service
	// header: the folder is a clause after the name on every document.
	FolderProperty = "MDL-DEPR105"
	// DocumentationProperty is `Documentation: '…'` in a regular expression,
	// task queue or scheduled event property list.
	DocumentationProperty = "MDL-DEPR106"
)

// entries is the registry. Append only: a code is never reused or renumbered,
// because scripts, CI allowlists and docs refer to it.
var entries = []Entry{
	{
		Code:      CreateOrReplace,
		Old:       "create or replace …",
		Canonical: "create or modify …",
		Rewrite:   Rewrite{Token: "replace", Replacement: "modify"},
		RemovedIn: 2,
		Note: "Not reported for `create or replace translations`, which replaces the " +
			"whole set. Under mdl 0 (no header) it is not reported for a view entity " +
			"(drops and recreates) or a user role / demo user (a plain create) either: " +
			"those warn MDL-V1-REPLACE01/02 instead, and are aliases from `mdl 1;` on.",
		Example:          "create or replace enumeration M.Color (Red 'Red');",
		CanonicalExample: "create or modify enumeration M.Color (Red 'Red');",
	},
	{
		Code:      Show,
		Old:       "show …",
		Canonical: "list …",
		Rewrite:   Rewrite{Token: "show", Replacement: "list"},
		RemovedIn: 2,
		Note: "Reported only for plurals and relationship queries, whose canonical " +
			"form is `list`. Forms that name a single thing (`show entity X`, " +
			"`show navigation`, `show project security`, …) become `describe`, and " +
			"session state (`show version`, `show status`) a REPL command; they are " +
			"not reported until those forms exist.",
		Example:          "show entities in M;",
		CanonicalExample: "list entities in M;",
	},
	{
		Code:      ListOperationFunctionForm,
		Old:       "$x = <operation>($List, …)",
		Canonical: "$x = <operation> $List …",
		Rewrite: Rewrite{Structural: "call form to statement form: head/tail $L; filter/find $L by Member = v " +
			"(when the condition has that shape) or where <expr>; sort $L by …; union/intersect $A with $B; " +
			"subtract($A, $B) -> subtract $B from $A; equals $A and $B; range($L, o, n) -> range $L offset o limit n"},
		RemovedIn: 2,
		Note: "A list operation is one Studio Pro activity whose operand is a variable, so the statement form " +
			"cannot nest. find(…) and contains(…) are not reported here: the call form is also the string " +
			"function, so they are version-gated instead (MDL-V1-LIST).",
		Example:          "create microflow M.F ($L: List of M.E) begin $H = head($L); end;",
		CanonicalExample: "create microflow M.F ($L: List of M.E) begin $H = head $L; end;",
	},
	{
		Code:      AggregateFunctionForm,
		Old:       "$n = <function>($List, …)",
		Canonical: "$n = <function> $List …",
		Rewrite: Rewrite{Structural: "call form to statement form: count $L; sum|average|minimum|maximum " +
			"$L by Attr (for $L.Attr) or of <expr>; all|any $L where <expr>; reduce $L from <initial> as <type> using <expr>"},
		RemovedIn:        2,
		Note:             "An aggregate is one Studio Pro Aggregate list activity whose operand is a variable.",
		Example:          "create microflow M.F ($L: List of M.E) begin $N = count($L); end;",
		CanonicalExample: "create microflow M.F ($L: List of M.E) begin $N = count $L; end;",
	},
	{
		Code:      UnstoredWidgetName,
		Old:       "row row1 { … } / column Name (…) — a name on an element Mendix stores none for",
		Canonical: "row { … } / column (…)",
		Rewrite:   Rewrite{Structural: "name out of the element: `row row1 {` becomes `row {`"},
		RemovedIn: 2,
		Note: "Mendix stores no name on a layout grid's row, a row's column, a data grid's column or control bar, " +
			"or a gallery's template or filter, so the name was never written and describe no longer invents one. " +
			"A data grid column is addressed as `grid column(Attr)` or `grid column('Caption')`.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { datagrid dg (DataSource: database from M.E) { column Name (Attribute: Name) } };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { datagrid dg (DataSource: database from M.E) { column (Attribute: Name) } };",
	},
	{
		Code:             ConsumedRestService,
		Old:              "rest client / rest clients",
		Canonical:        "consumed rest service / consumed rest services",
		Rewrite:          Rewrite{Structural: "document type name: `rest client` becomes `consumed rest service`, `rest clients` `consumed rest services`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a consumed REST service (R10). Every statement that names the type takes both spellings: create, alter, drop, describe, list, move.",
		Example:          "drop rest client M.Api;",
		CanonicalExample: "drop consumed rest service M.Api;",
	},
	{
		Code:             ConsumedODataService,
		Old:              "odata client / odata clients",
		Canonical:        "consumed odata service / consumed odata services",
		Rewrite:          Rewrite{Structural: "document type name: `odata client` becomes `consumed odata service`, `odata clients` `consumed odata services`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a consumed OData service (R10), including where an external entity names its source (`from consumed odata service M.Crm`).",
		Example:          "drop odata client M.Crm;",
		CanonicalExample: "drop consumed odata service M.Crm;",
	},
	{
		Code:             PublishedODataService,
		Old:              "odata service / odata services",
		Canonical:        "published odata service / published odata services",
		Rewrite:          Rewrite{Structural: "document type name: `odata service` becomes `published odata service`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a published OData service (R10), including in `grant|revoke access on published odata service`.",
		Example:          "drop odata service M.Api;",
		CanonicalExample: "drop published odata service M.Api;",
	},
	{
		Code:             TaskQueue,
		Old:              "queue / queues",
		Canonical:        "task queue / task queues",
		Rewrite:          Rewrite{Structural: "document type name: `queue` becomes `task queue`, `queues` `task queues`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls the document a task queue (R10). `call microflow … in queue M.Q` is an option of the call, not a document type name, and is unchanged.",
		Example:          "drop queue M.Jobs;",
		CanonicalExample: "drop task queue M.Jobs;",
	},
	{
		Code:             AppSecurity,
		Old:              "alter project security …",
		Canonical:        "alter app security …",
		Rewrite:          Rewrite{Structural: "security name: `project security` becomes `app security`"},
		RemovedIn:        2,
		Note:             "Studio Pro calls it App Security (R10). `show project security` becomes `describe app security` with the rest of R6, and is not reported here yet.",
		Example:          "alter project security demo users off;",
		CanonicalExample: "alter app security demo users off;",
	},
	{
		Code:             SettingsRuntime,
		Old:              "alter settings model …",
		Canonical:        "alter settings runtime …",
		Rewrite:          Rewrite{Structural: "section name: `model` becomes `runtime`"},
		RemovedIn:        2,
		Note:             "`runtime` is the App Settings tab that holds these values in Studio Pro (R10); `model` also collided with the agent-editor document type.",
		Example:          "alter settings model AfterStartupMicroflow = 'M.Startup';",
		CanonicalExample: "alter settings runtime AfterStartupMicroflow = 'M.Startup';",
	},
	{
		Code:      ReversedEntityGrant,
		Old:       "grant M.Role on M.E (rights) where '[xpath]'",
		Canonical: "grant rights on entity M.E to M.Role where [xpath]",
		Rewrite: Rewrite{Structural: "rights before `on entity`, roles after `to`, and the XPath out of its " +
			"string: `grant R on M.E (read *) where '[A = ''x'']'` becomes `grant read * on entity M.E to R where [A = 'x']`"},
		RemovedIn: 2,
		Note: "Every other grant names the right first and the role after `to`. XPath is written in [ ] " +
			"everywhere (R5), so the quotes inside it are no longer doubled. A string whose value is not " +
			"a bracketed XPath is left in place and reported by `fmt --upgrade`.",
		Example:          "grant M.User on M.Order (read *, write *) where '[Status = ''Open'']';",
		CanonicalExample: "grant read *, write * on entity M.Order to M.User where [Status = 'Open'];",
	},
	{
		Code:      QuotedTargetingXPath,
		Old:       "targeting [users|groups] xpath '[xpath]'",
		Canonical: "targeting [users|groups] xpath [xpath]",
		Rewrite:   Rewrite{Structural: "the XPath out of its string: `xpath '[Name = ''Admin'']'` becomes `xpath [Name = 'Admin']`"},
		RemovedIn: 2,
		Note: "XPath is written in [ ] everywhere (R5), so the quotes inside it are no longer doubled. " +
			"A string whose value is not a bracketed XPath is left in place and reported by `fmt --upgrade`.",
		Example:          "alter workflow M.WF set activity 'Review' targeting xpath '[Role = ''Manager'']';",
		CanonicalExample: "alter workflow M.WF set activity 'Review' targeting xpath [Role = 'Manager'];",
	},
	{
		Code:      OnErrorBraces,
		Old:       "on error [without rollback] { … }",
		Canonical: "on error [without rollback] begin … end error",
		Rewrite:   Rewrite{Structural: "`{` becomes `begin` and the closing `}` becomes `end error`"},
		RemovedIn: 2,
		Note: "Braces hold declarative children (widgets, operations, menu items); imperative flow is " +
			"`begin … end <keyword>`, as for `if`, `loop` and `while` (R2).",
		Example:          "create microflow M.F ($O: M.E) begin commit $O on error without rollback { log warning 'x'; }; end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin commit $O on error without rollback begin log warning 'x'; end error; end;",
	},
	{
		Code:      DollarArgumentName,
		Old:       "call microflow M.F($Param = expr)",
		Canonical: "call microflow M.F(Param = expr)",
		Rewrite:   Rewrite{Structural: "parameter name without its `$` (quoted when it is not an identifier or keyword)"},
		RemovedIn: 2,
		Note: "Every call site binds an argument as `Param = expression` (R4): call microflow, nanoflow, java " +
			"action, javascript action, external action, web service operation, execute database query, " +
			"send rest request, show page, and page/button actions and data sources.",
		Example:          "create microflow M.F ($O: M.E) begin call microflow M.G($Order = $O); end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin call microflow M.G(Order = $O); end;",
	},
	{
		Code:      ColonArgument,
		Old:       "show page M.P(Param: expr)",
		Canonical: "show page M.P(Param = expr)",
		Rewrite:   Rewrite{Structural: "colon as `=`: `Param: expr` -> `Param = expr`"},
		RemovedIn: 2,
		Note: "`:` sets a model property and `=` binds a runtime value (R3). An argument binds a value, so " +
			"it takes `=` wherever the call appears: show page, and page/button actions and data sources " +
			"(`Action: microflow M.F(Param = expr)`).",
		Example:          "create microflow M.F ($O: M.E) begin show page M.P(Order: $O); end;",
		CanonicalExample: "create microflow M.F ($O: M.E) begin show page M.P(Order = $O); end;",
	},
	{
		Code:      WorkflowStringArgument,
		Old:       "call microflow M.F with (Param = '<expression>')",
		Canonical: "call microflow M.F(Param = <expression>)",
		Rewrite:   Rewrite{Structural: "string list as a list after the callee, each string's content written as the bare expression"},
		RemovedIn: 2,
		Note: "In a workflow. The string form keeps its meaning — its content is the expression — so it is an alias, not a " +
			"change of meaning. A string whose content does not parse as an MDL expression is left in place " +
			"and reported by fmt --upgrade.",
		Example: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"call microflow M.F with (Order = '$WorkflowContext'); end workflow;",
		CanonicalExample: "create workflow M.W parameter $WorkflowContext: M.E begin " +
			"call microflow M.F(Order = $WorkflowContext); end workflow;",
	},
	{
		Code:      PositionalTemplateArguments,
		Old:       "objects [$a, $b] / parameters ['a', 'b']",
		Canonical: "with ({1} = $a, {2} = $b)",
		Rewrite:   Rewrite{Structural: "positional list as numbered placeholders: `objects [a, b]` -> `with ({1} = a, {2} = b)`"},
		RemovedIn: 2,
		Note: "One text-template form everywhere: show message, validation feedback, log, and REST " +
			"url and body templates.",
		Example:          "create microflow M.F ($N: String) begin show message 'Hi {1}' type Information objects [$N]; end;",
		CanonicalExample: "create microflow M.F ($N: String) begin show message 'Hi {1}' type Information with ({1} = $N); end;",
	},
}

func init() {
	entries = append(entries, r8Entries...)
	entries = append(entries, r9Entries...)
}

// r9Entries are R9's (ako/mxcli#755): documentation is a `/** … */` doc
// comment, the folder is a `folder '…'` clause, and a workflow activity's
// caption is `caption '…'`.
var r9Entries = []Entry{
	{
		Code:      DocumentationClause,
		Old:       "create constant|association|json structure|image collection … comment '…'",
		Canonical: "/** … */ before the statement",
		Rewrite: Rewrite{Structural: "documentation as a doc comment: the clause is deleted and its text " +
			"written as `/** … */` before the statement"},
		RemovedIn: 2,
		Note: "Documentation is a doc comment on every document. A statement that has both is reported, " +
			"not rewritten, and so is a text a doc comment cannot hold exactly (a `*/`, blank lines, " +
			"or space at the start or end of a line).",
		Example:          "create constant M.ApiUrl type string default 'https://x' comment 'Base URL';",
		CanonicalExample: "/** Base URL */\ncreate constant M.ApiUrl type string default 'https://x';",
	},
	{
		Code:             WorkflowCommentCaption,
		Old:              "<workflow activity> comment '…'",
		Canonical:        "<workflow activity> caption '…'",
		Rewrite:          Rewrite{Token: "comment", Replacement: "caption"},
		RemovedIn:        2,
		Note:             "The clause sets the caption Studio Pro shows on the activity; it was never a comment. Also on `end workflow` and an event sub-process's timer start.",
		Example:          "create workflow M.W parameter $WorkflowContext: M.E begin notification Ready comment 'Ready'; end workflow;",
		CanonicalExample: "create workflow M.W parameter $WorkflowContext: M.E begin notification Ready caption 'Ready'; end workflow;",
	},
	{
		Code:      FolderProperty,
		Old:       "create page|snippet|consumed rest service|… M.N (…, Folder: '…', …)",
		Canonical: "create page|snippet|consumed rest service|… M.N folder '…' (…)",
		Rewrite: Rewrite{Structural: "folder as a clause: the property is deleted from the list and written " +
			"as `folder '…'` after the name"},
		RemovedIn: 2,
		Note: "The folder is a clause after the name on every document, as for a microflow or an enumeration. " +
			"On a page, snippet, consumed REST service, consumed or published OData service and published " +
			"REST service.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Folder: 'Admin') { };",
		CanonicalExample: "create page M.P folder 'Admin' (Title: 'P', Layout: Atlas_Core.Atlas_Default) { };",
	},
	{
		Code:      DocumentationProperty,
		Old:       "Documentation: '…' in a regular expression, task queue or scheduled event",
		Canonical: "/** … */ before the statement",
		Rewrite: Rewrite{Structural: "documentation as a doc comment: the property is deleted from the list and " +
			"its text written as `/** … */` before the statement"},
		RemovedIn: 2,
		Note: "A statement that has both is reported, not rewritten, and so is a text a doc comment cannot " +
			"hold exactly.",
		Example:          "create regular expression M.Zip (Expression: '[0-9]{4}', Documentation: 'Dutch zip');",
		CanonicalExample: "/** Dutch zip */\ncreate regular expression M.Zip (Expression: '[0-9]{4}');",
	},
}

// r8Entries are R8's spellings (ako/mxcli#752). Kept apart from the list above
// only so the parallel phase-3 changes do not all edit its last lines.
var r8Entries = []Entry{
	{
		Code:      PageActionWord,
		Old:       "show_page, save_changes, close_page, microflow M.F, …",
		Canonical: "show page, save changes, close page, call microflow M.F, …",
		Rewrite: Rewrite{Structural: "page action as words: the underscore becomes a space (`show_page` -> " +
			"`show page`, also `save_changes`, `cancel_changes`, `close_page`, `create_object`, `open_link`, " +
			"`sign_out`, `complete_task`); `delete_object` -> `delete`; `microflow M.F` / `nanoflow M.F` -> " +
			"`call microflow M.F` / `call nanoflow M.F`"},
		RemovedIn: 2,
		Note: "The words are the ones a microflow uses for the same activity. A navigation menu's " +
			"`sign_out` is the same keyword. Arguments are unchanged.",
		Example:          "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { actionbutton b (Caption: 'Save', Action: sign_out) };",
		CanonicalExample: "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { actionbutton b (Caption: 'Save', Action: sign out) };",
	},
	{
		Code:      ErrorMessageKeyword,
		Old:       "error '…' / feedback '…' / error_message '…'",
		Canonical: "error message '…'",
		Rewrite: Rewrite{Structural: "message keyword as `error message`: `not null error '…'` (and after " +
			"`unique` or `required`), a validation rule's `feedback '…'`, and `error_message` / `errormessage`"},
		RemovedIn:        2,
		Note:             "One keyword for the text a user sees when a rule refuses a change.",
		Example:          "create entity M.E (Name: String(100) not null error 'Name is required');",
		CanonicalExample: "create entity M.E (Name: String(100) not null error message 'Name is required');",
	},
	{
		Code:      DeleteBehaviorClause,
		Old:       "delete_behavior …",
		Canonical: "on delete cascade|restrict|set null",
		Rewrite: Rewrite{Structural: "delete behaviour as the SQL referential action: " +
			"`delete_behavior cascade` / `delete_and_references` -> `on delete cascade`; " +
			"`prevent` / `delete_if_no_references` -> `on delete restrict`; " +
			"`delete_but_keep_references` -> `on delete set null`"},
		RemovedIn: 2,
		Note: "Also in `alter association … set delete_behavior …`. The three compound behaviour keywords " +
			"had three spellings each; the SQL referential actions have one.",
		Example:          "create association M.Order_Customer from M.Order to M.Customer type Reference delete_behavior prevent;",
		CanonicalExample: "create association M.Order_Customer from M.Order to M.Customer type Reference on delete restrict;",
	},
	{
		Code:             ReferenceSetUnderscore,
		Old:              "type reference_set",
		Canonical:        "type ReferenceSet",
		Rewrite:          Rewrite{Structural: "type name as Mendix writes it: `reference_set` -> `ReferenceSet`"},
		RemovedIn:        2,
		Example:          "create association M.Order_Tag from M.Order to M.Tag type reference_set;",
		CanonicalExample: "create association M.Order_Tag from M.Order to M.Tag type ReferenceSet;",
	},
	{
		Code:             ReturnsNone,
		Old:              "rest call … returns none",
		Canonical:        "rest call … returns nothing",
		Rewrite:          Rewrite{Token: "none", Replacement: "nothing"},
		RemovedIn:        2,
		Example:          "create microflow M.F () begin rest call get 'https://example.com' returns none; end;",
		CanonicalExample: "create microflow M.F () begin rest call get 'https://example.com' returns nothing; end;",
	},
}

// All returns every registered entry, in code order.
func All() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// Lookup returns the entry for code.
func Lookup(code string) (Entry, bool) {
	for _, e := range entries {
		if e.Code == code {
			return e, true
		}
	}
	return Entry{}, false
}

// IsDeprecationCode reports whether a violation rule ID is a registry code.
func IsDeprecationCode(ruleID string) bool {
	return strings.HasPrefix(ruleID, "MDL-DEPR")
}

// Policy is what `check` and `exec` do with a deprecated spelling.
type Policy int

const (
	// Warn reports it and carries on. The default.
	Warn Policy = iota
	// Error fails the run, for CI over docs, skills and examples.
	Error
)

// ParsePolicy reads the --deprecations flag value.
func ParsePolicy(s string) (Policy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "warn":
		return Warn, nil
	case "error":
		return Error, nil
	}
	return Warn, fmt.Errorf("invalid --deprecations value %q: want warn or error", s)
}
