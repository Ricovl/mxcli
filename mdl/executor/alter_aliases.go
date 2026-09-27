// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The old spellings of ALTER PAGE / SNIPPET / LAYOUT are aliases of the generic
// ALTER (ADR-0012 decision 2): they parse to the identical operation and warn
// (ADR-0011: an old form keeps working through the alias window, and says what
// replaces it).
//
// Each entry has the shape the deprecation registry (ako/mxcli#709) takes —
// code, old form, canonical form, mechanical rewrite, language version it is
// removed in — so the registry absorbs this table rather than re-deriving it.
// Until it lands the codes are provisional and numbered from 101, clear of the
// registry's seed entries. The grammar marks each alias alternative with
// `// alias: <code>`; TestAlterAliasGrammarMarkersMatchTable pins the two
// together, in both directions.
type alterAlias struct {
	Code      string // MDL-DEPRnnn
	Spelling  string // ast.AlterAlias*
	Old       string // the old form, as written
	Canonical string // what replaces it
	Rewrite   string // the mechanical rewrite, for `fmt --upgrade`
	RemovedIn string // the MDL language version that drops the alias
}

var alterAliases = []alterAlias{
	{
		Code:      "MDL-DEPR101",
		Spelling:  ast.AlterAliasSetEquals,
		Old:       "set Key = value [on target]  /  set (Key = value, …) [on target]",
		Canonical: "set (Key: value, …) [on target]",
		Rewrite:   "put the assignments in parentheses and write each `=` as `:`",
		RemovedIn: "mdl 2",
	},
	{
		Code:      "MDL-DEPR102",
		Spelling:  ast.AlterAliasSetUnparenthesised,
		Old:       "set Key: value [on target]",
		Canonical: "set (Key: value) [on target]",
		Rewrite:   "put the assignment in parentheses",
		RemovedIn: "mdl 2",
	},
	{
		Code:      "MDL-DEPR103",
		Spelling:  ast.AlterAliasDropWidget,
		Old:       "drop widget a, b",
		Canonical: "drop a, b",
		Rewrite:   "remove the word `widget`",
		RemovedIn: "mdl 2",
	},
}

func alterAliasFor(spelling string) (alterAlias, bool) {
	for _, a := range alterAliases {
		if a.Spelling == spelling {
			return a, true
		}
	}
	return alterAlias{}, false
}

// validateAlterAliases warns on every old ALTER spelling a statement uses, once
// per operation. A warning, never an error: both spellings build the identical
// operation, and scripts in the wild use the old ones.
func validateAlterAliases(stmt ast.Statement) []linter.Violation {
	s, ok := stmt.(*ast.AlterPageStmt)
	if !ok {
		return nil
	}
	kind := strings.ToLower(s.ContainerType)
	if kind == "" {
		kind = "page"
	}
	var out []linter.Violation
	for _, op := range s.Operations {
		var spelling string
		switch o := op.(type) {
		case *ast.SetPropertyOp:
			spelling = o.Legacy
		case *ast.DropWidgetOp:
			spelling = o.Legacy
		}
		if spelling == "" {
			continue
		}
		a, known := alterAliasFor(spelling)
		if !known {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   a.Code,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("alter %s %s: `%s` is the old spelling of `%s`",
				kind, s.PageName.String(), a.Old, a.Canonical),
			Location: linter.Location{
				Module:       s.PageName.Module,
				DocumentType: kind,
				DocumentName: s.PageName.Name,
			},
			Suggestion: fmt.Sprintf("Write `%s` (%s). Both build the identical change; the old form is removed in %s.",
				a.Canonical, a.Rewrite, a.RemovedIn),
		})
	}
	return out
}

// validateAlterPageAddresses refuses, with no project needed, a target whose
// address FORM a page cannot use: a quoted caption, an @n, a path of more than
// two names. The generic grammar accepts every address form any document type
// uses, and the page family's resolver refuses these whatever the page holds —
// so `check` can say so up front instead of `exec` stopping mid-script. The
// rule is the resolver's own (backend.CheckPageAlterTarget), not a copy.
func validateAlterPageAddresses(stmt ast.Statement) []linter.Violation {
	s, ok := stmt.(*ast.AlterPageStmt)
	if !ok {
		return nil
	}
	kind := strings.ToLower(s.ContainerType)
	if kind == "" {
		kind = "page"
	}
	var out []linter.Violation
	for _, op := range s.Operations {
		for _, ref := range alterPageOperationTargets(op) {
			if err := backend.CheckPageAlterTarget(alterTargetOf(ref)); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-ALTER01",
					Severity: linter.SeverityError,
					Message:  fmt.Sprintf("alter %s %s: %v", kind, s.PageName.String(), err),
					Location: linter.Location{
						Module:       s.PageName.Module,
						DocumentType: kind,
						DocumentName: s.PageName.Name,
					},
				})
			}
		}
	}
	return out
}
