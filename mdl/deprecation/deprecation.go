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
