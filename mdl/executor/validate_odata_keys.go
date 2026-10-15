// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// What Mendix requires of a published entity's KEY (mendixlabs/mxcli#1426).
//
// Measured with mx 11.6.6 on a copy of testdata/expr-checker, one service per
// case in one app (mxbuild reports them all):
//
//	no KEY (persistent, non-persistent, view; no expose; expose (*))  -> CE6585
//	one KEY, persistable, no unique validation rule                   -> CE6624
//	  ... with only `not null`, or only an index on it                -> CE6624
//	  ... on OData3 and on OData4 alike                               -> CE6624
//	one KEY, persistable, `unique` (OData3 and OData4)                -> 0 errors
//	composite KEY, OData4, no unique rule on either part              -> 0 errors
//	one KEY on a non-persistent entity / a view entity, no rule       -> 0 errors
//
// So CE6624 is about a single-attribute key on a persistable entity, and only a
// unique VALIDATION RULE satisfies it — a Mendix index has no unique flag, and
// is not one. A composite key needs no rule, but is OData4-only (CE7238), which
// is not this check's business. Non-persistable entities cannot carry a
// validation rule at all (CE0070), which is why they are exempt.

// ValidateODataKeys flags a published entity with no KEY (MDL-ODATA08) or with a
// single KEY attribute that has no unique validation rule (MDL-ODATA09).
//
// MDL-ODATA08 needs nothing but the statement. MDL-ODATA09 needs the entity —
// from the script first, the project second — and says nothing about an entity
// it cannot resolve, about an inherited key attribute, or about an entity the
// script alters, rather than guess in the confident direction.
func ValidateODataKeys(prog *ast.Program, projectPath string) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	var singles []singleKey
	for _, stmt := range prog.Statements {
		svc, ok := stmt.(*ast.CreateODataServiceStmt)
		if !ok {
			continue
		}
		for _, e := range svc.Entities {
			if e == nil || e.Entity.Name == "" {
				continue
			}
			where := fmt.Sprintf("publish entity %s in odata service %s", e.Entity.String(), svc.Name.String())
			loc := linter.Location{
				Module:       svc.Name.Module,
				DocumentType: "published odata service",
				DocumentName: svc.Name.Name,
			}
			var keys []string
			for _, m := range e.Members {
				if m != nil && m.IsPartOfKey {
					keys = append(keys, m.Name)
				}
			}
			switch len(keys) {
			case 0:
				out = append(out, noKeyViolation(where, loc, e))
			case 1:
				singles = append(singles, singleKey{where: where, loc: loc, entity: e.Entity.String(), attr: keys[0]})
			}
		}
	}
	if len(singles) == 0 {
		return out
	}
	return append(out, nonUniqueKeyViolations(prog, projectPath, singles)...)
}

type singleKey struct {
	where  string
	loc    linter.Location
	entity string
	attr   string
}

func noKeyViolation(where string, loc linter.Location, e *ast.PublishedEntityDef) linter.Violation {
	example := "Name"
	for _, m := range e.Members {
		if m != nil && m.Name != "" {
			example = m.Name
			break
		}
	}
	return linter.Violation{
		RuleID:   "MDL-ODATA08",
		Severity: linter.SeverityError,
		Location: loc,
		Message: fmt.Sprintf("%s: no exposed member is marked KEY, and Mendix requires a published entity "+
			"to have one (CE6585 \"Published entity '%s' must have a key defined.\")", where, e.Entity.String()),
		Suggestion: fmt.Sprintf("Mark the member that identifies a row, e.g. expose (%s (KEY), ...). On a "+
			"persistent entity a single KEY attribute needs a unique validation rule "+
			"(`%s: String(200) unique`); a composite key (two or more KEY members, OData4 only) does not.",
			example, example),
	}
}

// nonUniqueKeyViolations reports MDL-ODATA09 for single keys on a persistable
// entity whose own key attribute carries no unique rule.
func nonUniqueKeyViolations(prog *ast.Program, projectPath string, keys []singleKey) []linter.Violation {
	res := newPersistabilityResolver(prog, projectPath)
	defer res.close()

	// An entity the script alters may gain or lose the rule on the way; its
	// final state is not what either definition says, so leave it alone.
	altered := map[string]bool{}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.AlterEntityStmt); ok {
			altered[s.Name.String()] = true
		}
	}

	var out []linter.Violation
	for _, k := range keys {
		if altered[k.entity] {
			continue
		}
		facts, ok := res.lookup(k.entity)
		if !ok || facts.view || facts.uniqueAttrs == nil {
			continue
		}
		unique, own := facts.uniqueAttrs[k.attr]
		if !own || unique {
			// Not an attribute of the entity itself: inherited, an association,
			// or a typo another check reports. None is decidable here.
			continue
		}
		if persistable, known := res.persistable(k.entity); !known || !persistable {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-ODATA09",
			Severity: linter.SeverityError,
			Location: k.loc,
			Message: fmt.Sprintf("%s: KEY %s has no unique validation rule, which Mendix requires of a "+
				"single key attribute on a persistent entity (CE6624 \"Add a unique validation rule to "+
				"attribute '%s' of entity '%s' to be able to use it as the key.\")",
				k.where, k.attr, k.attr, k.entity),
			Suggestion: fmt.Sprintf("Declare the attribute unique — `%s: String(200) unique` in the entity, or "+
				"`alter entity %s modify attribute %s unique` for a stored one. Not null or an index "+
				"does not count.", k.attr, k.entity, k.attr),
		})
	}
	return out
}

// scriptUniqueAttrs records which of a script entity's own attributes are
// declared unique.
func scriptUniqueAttrs(s *ast.CreateEntityStmt) map[string]bool {
	out := make(map[string]bool, len(s.Attributes))
	for _, a := range s.Attributes {
		out[a.Name] = a.Unique
	}
	return out
}

// isViewSource reports whether a stored entity's rows come from an OQL view.
func isViewSource(source string) bool {
	return strings.Contains(source, "OqlViewEntitySource")
}
