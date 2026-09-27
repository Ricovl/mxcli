// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Text the source says and the model does not keep (ako/mxcli#706). Both rules
// here are warnings, not refusals: the element itself is written correctly and
// only the words are lost. The forms that stored a WRONG model — throw,
// float/currency/date, the parenthesised association — are refused by the
// visitor instead (visitor_silent_drops.go).

// A Mendix index is anonymous: DomainModels$Index holds its columns and nothing
// that names it. MDL accepts a name in three spellings — in the entity body,
// `alter entity … add index Name (…)` and `create index Name on E (…)` — and
// used to discard it without a word, so the script said something the model
// does not keep: `describe` prints the index back as `index (…)`, and
// `drop index Name` cannot find it (ako/mxcli#706).
//
// A warning rather than a refusal: the index itself is written correctly, and
// the name is the required part of `create index`, whose retirement belongs to
// the deprecation registry, not to this check.
const indexNameRule = "MDL-IDX01"

// validateIndexNames reports every index in stmt that was given a name.
func validateIndexNames(stmt ast.Statement) []linter.Violation {
	var entity ast.QualifiedName
	var named []ast.Index
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		entity = s.Name
		named = s.Indexes
	case *ast.AlterEntityStmt:
		if s.Operation != ast.AlterEntityAddIndex || s.Index == nil {
			return nil
		}
		entity = s.Name
		named = []ast.Index{*s.Index}
	default:
		return nil
	}

	var out []linter.Violation
	for _, idx := range named {
		if idx.Name == "" {
			continue
		}
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = c.Name
			if c.Descending {
				cols[i] += " desc"
			}
		}
		out = append(out, linter.Violation{
			RuleID:   indexNameRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("index name %q on %s is not stored — a Mendix index has no name, "+
				"so describe shows it as `index (…)` and `drop index %s` will not find it",
				idx.Name, entity.String(), idx.Name),
			Location: linter.Location{
				Module:       entity.Module,
				DocumentType: "entity",
				DocumentName: entity.Name,
			},
			Suggestion: fmt.Sprintf("write it without the name: alter entity %s add index (%s); "+
				"select it for a drop by its columns: drop index (%s)",
				entity.String(), strings.Join(cols, ", "), strings.Join(cols, ", ")),
		})
	}
	return out
}

// Enumerations$EnumerationValue has Name, Caption and Image and no
// documentation, so a `/** … */` written before a value was read and written
// nowhere. The enumeration's own doc comment, before `create`, is stored.
const enumValueDocRule = "MDL-ENUMDOC01"

// validateEnumValueDocs reports every enumeration value that carries a doc comment.
func validateEnumValueDocs(stmt ast.Statement) []linter.Violation {
	s, ok := stmt.(*ast.CreateEnumerationStmt)
	if !ok {
		return nil
	}
	var out []linter.Violation
	for _, v := range s.Values {
		if v.Documentation == "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   enumValueDocRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf("the doc comment on enumeration value %s.%s is not stored — "+
				"Mendix keeps no documentation on an enumeration value", s.Name.String(), v.Name),
			Location: linter.Location{
				Module:       s.Name.Module,
				DocumentType: "enumeration",
				DocumentName: s.Name.Name,
			},
			Suggestion: "use an ordinary comment (-- …) for a note on a value, or put the text " +
				"in the enumeration's own /** … */ before `create enumeration`",
		})
	}
	return out
}
