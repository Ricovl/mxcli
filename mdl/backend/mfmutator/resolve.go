// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"fmt"
	"sort"
	"strings"
)

// NotFoundError reports a target that addresses no activity.
type NotFoundError struct {
	Target Target
	// Hint lists what the target could have meant, e.g. the output variables
	// the flow does have. May be empty.
	Hint string
}

func (e *NotFoundError) Error() string {
	msg := fmt.Sprintf("no activity matches %s %s", e.Target.Kind, e.Target.Text)
	if e.Hint != "" {
		msg += "; " + e.Hint
	}
	return msg
}

// AmbiguousError reports a target that addresses more than one activity and
// carries no ordinal (or an ordinal past the last match). It lists each match
// under the address that selects it.
type AmbiguousError struct {
	Target  Target
	Matches []Candidate
}

func (e *AmbiguousError) Error() string {
	var b strings.Builder
	if e.Target.Ordinal > len(e.Matches) {
		fmt.Fprintf(&b, "%s %s @%d: there are only %d matches:", e.Target.Kind, e.Target.Text, e.Target.Ordinal, len(e.Matches))
	} else {
		fmt.Fprintf(&b, "%s %s matches %d activities; add an ordinal to choose one:", e.Target.Kind, e.Target.Text, len(e.Matches))
	}
	for i, c := range e.Matches {
		fmt.Fprintf(&b, "\n  %s @%d  -- %s", e.Target.Text, i+1, describeCandidate(c))
	}
	return b.String()
}

// Resolve returns the one candidate t addresses. Zero matches is a
// NotFoundError; more than one without an ordinal, or an ordinal past the
// last match, is an AmbiguousError. It never picks among matches on its own.
func Resolve(cands []Candidate, t Target) (Candidate, error) {
	idx := matchIndices(cands, t)
	switch {
	case len(idx) == 0:
		return Candidate{}, &NotFoundError{Target: t, Hint: notFoundHint(cands, t)}
	case t.Ordinal > len(idx):
		return Candidate{}, &AmbiguousError{Target: t, Matches: pick(cands, idx)}
	case t.Ordinal > 0:
		return cands[idx[t.Ordinal-1]], nil
	case len(idx) == 1:
		return cands[idx[0]], nil
	default:
		return Candidate{}, &AmbiguousError{Target: t, Matches: pick(cands, idx)}
	}
}

// ResolveText parses text as a target and resolves it.
func ResolveText(cands []Candidate, text string) (Candidate, error) {
	t, err := ParseTarget(text)
	if err != nil {
		return Candidate{}, err
	}
	return Resolve(cands, t)
}

func matchIndices(cands []Candidate, t Target) []int {
	var idx []int
	for i := range cands {
		if cands[i].matches(t) {
			idx = append(idx, i)
		}
	}
	return idx
}

func pick(cands []Candidate, idx []int) []Candidate {
	out := make([]Candidate, len(idx))
	for i, j := range idx {
		out[i] = cands[j]
	}
	return out
}

// notFoundHint names the addresses of the same kind that do exist, which is
// usually enough to spot a typo, and says when an anchored pattern only
// failed because the statement goes on.
func notFoundHint(cands []Candidate, t Target) string {
	switch t.Kind {
	case ByOutputVariable:
		return listHint("output variables", cands, func(c Candidate) string {
			if c.OutputVariable == "" {
				return ""
			}
			return "$" + c.OutputVariable
		})
	case ByCaption:
		return listHint("captions", cands, func(c Candidate) string {
			if c.Caption == "" {
				return ""
			}
			return quote(c.Caption)
		})
	default:
		open := append(append([]Token{}, t.Pattern...), Token{Kind: TokStar, Text: "*"})
		for i := range cands {
			if cands[i].matchesPattern(open) {
				return fmt.Sprintf("patterns match the whole statement: end it with * to match %q", cands[i].Statement)
			}
		}
		return ""
	}
}

func listHint(what string, cands []Candidate, name func(Candidate) string) string {
	seen := map[string]bool{}
	var names []string
	for _, c := range cands {
		if n := name(c); n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "the microflow has no " + what
	}
	sort.Strings(names)
	return "the microflow's " + what + " are " + strings.Join(names, ", ")
}

// Handle returns the preferred address of cands[i]: its output variable, else
// its caption, else its statement — the first of those that selects it alone.
// When none is unique it returns the first available form with the ordinal
// that selects it. It returns "" for a candidate nothing can address (an end
// event describe renders as nothing).
//
// Handles are what `describe … with handles` prints, so each one is checked
// by resolving it rather than assumed: a statement containing `*` (a
// multiplication) reads back as a wildcard pattern and may select more than
// its own activity.
func Handle(cands []Candidate, i int) string {
	c := cands[i]
	var forms []string
	if c.OutputVariable != "" {
		forms = append(forms, "$"+c.OutputVariable)
	}
	if c.Caption != "" && !strings.ContainsAny(c.Caption, "\r\n") {
		// A handle is printed on one comment line; a multi-line caption
		// cannot be, so such an activity is addressed by its statement.
		forms = append(forms, quote(c.Caption))
	}
	if c.Statement != "" {
		forms = append(forms, c.Statement)
	}

	fallback := ""
	for _, form := range forms {
		t, err := ParseTarget(form)
		if err != nil || t.Ordinal != 0 {
			// A statement ending in `@<digits>` would read back with an
			// ordinal; it cannot serve as its own handle.
			continue
		}
		idx := matchIndices(cands, t)
		pos := -1
		for k, j := range idx {
			if j == i {
				pos = k
				break
			}
		}
		if pos < 0 {
			continue
		}
		if len(idx) == 1 {
			return form
		}
		if fallback == "" {
			fallback = fmt.Sprintf("%s @%d", form, pos+1)
		}
	}
	return fallback
}
