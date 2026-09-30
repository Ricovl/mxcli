// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"
)

// A three-part name in an XPath constraint is one of two things, and what it
// is decides what is stored (ako/mxcli#874):
//
//   - Module.Entity.Attribute — an attribute of the entity the constraint is
//     evaluated on. XPath names an attribute bare, so it is stored as
//     `Attribute`; Studio Pro reports the qualified spelling as CE0161.
//   - Module.Enumeration.Value — an enumeration value. XPath compares an
//     enumeration attribute with the value's name as a string, so it is stored
//     as `'Value'` (normalizeXPathEnumRefs).
//
// Every writer used to take the second reading for both, so a qualified
// attribute became a string literal and `[M.Emp.Name = 'y']` compared two
// constants: a constraint that passes every check and filters nothing.
//
// Which entity a name is qualified with is read off the constraint's context,
// not the model, so both the writer and the comparison create or modify makes
// against describe's form reach the same answer with no project open: the
// constrained entity itself, and every entity named as a step of a path in the
// constraint (a predicate on that step is evaluated on it). A three-part name
// qualified with anything else keeps the enumeration reading.

// xpathQualifiedNameRe matches a run of dot-joined identifiers; the callers
// look at how many parts it has and what surrounds it.
var xpathQualifiedNameRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+`)

// storedXPathConstraint is the constraint an XPath writer stores for xpath
// evaluated on entity: qualified attributes bare, enumeration values as string
// literals. String literals are data and are left as written.
func storedXPathConstraint(xpath, entity string) string {
	return normalizeXPathEnumRefs(resolveXPathMemberNames(xpath, entity))
}

// resolveXPathMemberNames rewrites each Module.Entity.Attribute whose
// Module.Entity is entity, or an entity step of the constraint, to Attribute.
func resolveXPathMemberNames(xpath, entity string) string {
	if !strings.Contains(xpath, ".") {
		return xpath
	}
	owners := map[string]bool{}
	if entity != "" {
		owners[entity] = true
	}
	forEachXPathName(xpath, func(name string) {
		if strings.Count(name, ".") == 1 {
			owners[name] = true
		}
	})
	return rewriteXPathNames(xpath, func(name string) string {
		if strings.Count(name, ".") != 2 {
			return name
		}
		dot := strings.LastIndex(name, ".")
		if owners[name[:dot]] {
			return name[dot+1:]
		}
		return name
	})
}

// forEachXPathName calls fn with every qualified name outside a string literal.
func forEachXPathName(xpath string, fn func(string)) {
	rewriteXPathNames(xpath, func(name string) string {
		fn(name)
		return name
	})
}

// rewriteXPathNames replaces each qualified name outside a string literal with
// what fn returns for it. A name glued to a `$` or `%` (a variable member, a
// `[%Token%]`) is not a model name and is passed over.
func rewriteXPathNames(xpath string, fn func(string) string) string {
	var b strings.Builder
	b.Grow(len(xpath))
	rewrite := func(seg string) {
		prev := 0
		for _, loc := range xpathQualifiedNameRe.FindAllStringIndex(seg, -1) {
			start, end := loc[0], loc[1]
			b.WriteString(seg[prev:start])
			if start > 0 && strings.ContainsRune("$%", rune(seg[start-1])) {
				b.WriteString(seg[start:end])
			} else {
				b.WriteString(fn(seg[start:end]))
			}
			prev = end
		}
		b.WriteString(seg[prev:])
	}
	for i := 0; i < len(xpath); {
		if xpath[i] != '\'' {
			j := strings.IndexByte(xpath[i:], '\'')
			if j < 0 {
				rewrite(xpath[i:])
				break
			}
			rewrite(xpath[i : i+j])
			i += j
			continue
		}
		end := xpathLiteralEnd(xpath, i)
		b.WriteString(xpath[i:end])
		i = end
	}
	return b.String()
}
