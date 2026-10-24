// SPDX-License-Identifier: Apache-2.0

package visitor

import "strings"

// XPathGroupSpan locates one top-level predicate group inside the constraint it
// was found in: constraint[Start:End] is the group, brackets included.
type XPathGroupSpan struct {
	Start, End int
}

// SplitXPathPredicateGroups splits a stored XPathConstraint into its top-level
// predicate groups, each returned with its enclosing brackets. Mendix concatenates
// sibling groups — `[a][b][c]`, often separated by newlines — and the whole string
// is what the BSON field holds.
//
// Splitting on "][" is not enough: a group may nest brackets of its own
// (`[Mod.Assoc/Mod.Entity[EndDate = $Limit]]`), and a string literal may contain a
// bracket (`[Name = 'a]b']`). Both appear in real projects, and mishandling either
// silently changes the meaning of a query (mendixlabs/mxcli#772). This tracks
// nesting depth and quoting instead.
//
// Returns nil when the input is not a well-formed sequence of bracket groups —
// unbalanced, empty, or with content outside the brackets — so callers can fall
// back to using the string verbatim rather than emit something they invented.
//
// The groups alone do not reproduce the input: the whitespace between them is
// dropped. A caller that edits a group and writes the constraint back must use
// XPathPredicateGroupSpans and splice, or it rewrites the layout of every
// constraint it touches (ako/mxcli#1506).
func SplitXPathPredicateGroups(constraint string) []string {
	spans := XPathPredicateGroupSpans(constraint)
	if spans == nil {
		return nil
	}
	groups := make([]string, len(spans))
	for i, sp := range spans {
		groups[i] = constraint[sp.Start:sp.End]
	}
	return groups
}

// XPathPredicateGroupSpans is SplitXPathPredicateGroups returning where each
// group sits in constraint rather than a copy of it. The offsets index the string
// exactly as given — leading whitespace included — so everything outside the
// spans is the layout between groups, which an edit must carry over verbatim.
func XPathPredicateGroupSpans(constraint string) []XPathGroupSpan {
	trimmed := strings.TrimSpace(constraint)
	if trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return nil
	}

	s := constraint
	var spans []XPathGroupSpan
	var depth int
	var start int
	var inQuote bool

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			// Mendix escapes a quote inside a literal by doubling it.
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inQuote = false
			}
			continue
		}
		switch c {
		case '\'':
			inQuote = true
		case '[':
			if depth == 0 {
				// Anything before or between groups must be whitespace only.
				if strings.TrimSpace(s[start:i]) != "" {
					return nil
				}
				start = i
			}
			depth++
		case ']':
			depth--
			if depth < 0 {
				return nil
			}
			if depth == 0 {
				spans = append(spans, XPathGroupSpan{Start: start, End: i + 1})
				start = i + 1
			}
		}
	}

	if depth != 0 || inQuote {
		return nil
	}
	if strings.TrimSpace(s[start:]) != "" {
		return nil
	}
	if len(spans) == 0 {
		return nil
	}
	return spans
}
