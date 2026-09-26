// SPDX-License-Identifier: Apache-2.0

// Package mfmutator holds the engine-agnostic microflow ALTER logic, alongside
// pagemutator and wfmutator. It starts with the piece every microflow patch
// needs first: finding the activity an operation is aimed at.
//
// # Content addressing
//
// A page addresses a widget by its name. Microflow activities have no names,
// so an `alter microflow` target names an activity by what it is
// (ADR-0012, decision 2), in this order of preference:
//
//  1. its output variable: `$Lines`;
//  2. its caption: `'Email is valid?'` — for splits, and for activities whose
//     caption is custom rather than generated;
//  3. a statement pattern, where `*` stands for any run of tokens:
//     `commit $Order`, `log * node 'Debug' *`.
//
// When an address matches more than one activity, that is an error listing
// every match with the ordinal (`@2`) that picks it. The resolver never
// guesses: a patch applied to the wrong activity of a Studio Pro-authored flow
// is a silent change of behaviour, which is worse than a refusal.
//
// # What is matched against what
//
// The resolver works on Candidates: one per activity in the stored object
// collection (loop bodies included), each carrying its output variable, its
// custom caption and its statement as `describe` renders it. Rendering a
// statement is the executor's job, so this package takes the text as input and
// stays free of any executor dependency; the order of the candidates is the
// order ordinals count in, which the caller sets to the order `describe` prints
// the activities.
package mfmutator

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Kind says which of the three addressing forms a Target uses.
type Kind int

const (
	// ByOutputVariable addresses the activity that outputs `$Name`.
	ByOutputVariable Kind = iota
	// ByCaption addresses a split or activity by its (custom) caption.
	ByCaption
	// ByPattern addresses an activity by its statement, with `*` wildcards.
	ByPattern
)

func (k Kind) String() string {
	switch k {
	case ByOutputVariable:
		return "output variable"
	case ByCaption:
		return "caption"
	default:
		return "statement pattern"
	}
}

// Target is a parsed content address.
type Target struct {
	// Text is the address as written, ordinal excluded; used in messages.
	Text string
	Kind Kind
	// Variable is the output variable name without `$` (ByOutputVariable).
	Variable string
	// Caption is the unquoted caption (ByCaption).
	Caption string
	// Pattern is the tokenised statement pattern (ByPattern). A token whose
	// text is `*` is a wildcard.
	Pattern []Token
	// Ordinal is the 1-based `@n` suffix, or 0 when none was written.
	Ordinal int
}

// ParseTarget parses a content address: `$Var`, `'Caption'` or a statement
// pattern, each optionally followed by an ordinal `@n`.
//
// A lone variable is always an output-variable address and a lone string
// always a caption address; anything longer is a statement pattern. The forms
// do not fall back on one another — a `$Var` that outputs nothing is an error,
// not a pattern search — so an address means one thing only.
func ParseTarget(text string) (Target, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimSpace(s)

	var t Target
	if at := strings.LastIndex(s, "@"); at >= 0 {
		digits := strings.TrimSpace(s[at+1:])
		if digits != "" && isDigits(digits) && !insideString(s, at) {
			n, err := strconv.Atoi(digits)
			if err != nil || n < 1 {
				return Target{}, fmt.Errorf("target %q: ordinal @%s must be 1 or more", text, digits)
			}
			t.Ordinal = n
			s = strings.TrimSpace(s[:at])
		}
	}
	if s == "" {
		return Target{}, fmt.Errorf("target %q is empty: address an activity by $variable, 'caption' or statement pattern", text)
	}
	t.Text = s

	toks, err := Tokenize(s)
	if err != nil {
		return Target{}, fmt.Errorf("target %q: %w", text, err)
	}
	switch {
	case len(toks) == 1 && toks[0].Kind == TokVariable:
		t.Kind = ByOutputVariable
		t.Variable = strings.TrimPrefix(toks[0].Text, "$")
	case len(toks) == 1 && toks[0].Kind == TokString:
		t.Kind = ByCaption
		t.Caption = toks[0].Text
	default:
		t.Kind = ByPattern
		t.Pattern = toks
	}
	return t, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// insideString reports whether byte offset i of s falls inside a '…' literal.
func insideString(s string, i int) bool {
	in := false
	for j := 0; j < i; j++ {
		if s[j] == '\'' {
			in = !in
		}
	}
	return in
}

// Candidate is one addressable activity of a microflow.
type Candidate struct {
	ID     model.ID
	Object microflows.MicroflowObject
	// OutputVariable is the variable the activity outputs, without `$`, or "".
	OutputVariable string
	// Caption is the custom caption, or "" when the caption is generated.
	Caption string
	// Statement is the activity's statement as `describe` prints it, folded
	// onto one line. Empty for an object describe prints as nothing (a void
	// flow's end event). A handle built from the statement uses this form.
	Statement string
	// Alternates are other renderings of the same activity a pattern also
	// matches. Describe does not always print the stored statement verbatim —
	// an `if` whose then-branch is empty is printed with the condition negated
	// and the branches swapped — so the stored form is kept here and both
	// match: a reader may have either in front of them.
	Alternates []string

	forms [][]Token // tokenised Statement and Alternates, built lazily
}

// SetPrinted records the statement as describe actually printed it. When it
// differs from the rendering the candidate was built with, that rendering
// becomes an alternate rather than being lost.
func (c *Candidate) SetPrinted(printed string) {
	printed = normalizeStatement(printed)
	if printed == "" || printed == c.Statement {
		return
	}
	if c.Statement != "" {
		c.Alternates = append(c.Alternates, c.Statement)
	}
	c.Statement = printed
	c.forms = nil
}

// NewCandidate builds the Candidate for obj, taking its statement text from
// render. It returns false for objects that are not addressable activities
// (start events, merges, annotations).
func NewCandidate(obj microflows.MicroflowObject, render func(microflows.MicroflowObject) string) (Candidate, bool) {
	c := Candidate{Object: obj}
	switch o := obj.(type) {
	case *microflows.ActionActivity:
		c.OutputVariable = OutputVariable(o.Action)
		if !o.AutoGenerateCaption {
			c.Caption = o.Caption
		}
	case *microflows.ExclusiveSplit:
		c.Caption = o.Caption
	case *microflows.InheritanceSplit:
		c.Caption = o.Caption
	case *microflows.LoopedActivity, *microflows.EndEvent, *microflows.ErrorEvent,
		*microflows.BreakEvent, *microflows.ContinueEvent:
	default:
		return Candidate{}, false
	}
	c.ID = obj.GetID()
	if render != nil {
		c.Statement = normalizeStatement(render(obj))
	}
	return c, true
}

// Collect returns a Candidate for every addressable activity in oc, loop
// bodies included (an address is unique across the whole microflow, not per
// scope), in storage order. Callers that show ordinals to a user reorder the
// result with OrderBy.
func Collect(oc *microflows.MicroflowObjectCollection, render func(microflows.MicroflowObject) string) []Candidate {
	var out []Candidate
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			if obj == nil {
				continue
			}
			if c, ok := NewCandidate(obj, render); ok {
				out = append(out, c)
			}
			if loop, ok := obj.(*microflows.LoopedActivity); ok {
				walk(loop.ObjectCollection)
			}
		}
	}
	walk(oc)
	return out
}

// OrderBy sorts cands by rank (ascending); candidates without a rank keep
// their relative order after all ranked ones. The executor ranks by the line
// `describe` prints each activity on, so ordinals count the way a reader does.
func OrderBy(cands []Candidate, rank map[model.ID]int) []Candidate {
	out := make([]Candidate, 0, len(cands))
	var unranked []Candidate
	for _, c := range cands {
		if _, ok := rank[c.ID]; ok {
			out = append(out, c)
		} else {
			unranked = append(unranked, c)
		}
	}
	// Insertion sort keeps it stable without pulling in sort.SliceStable's
	// closure for a list of a few dozen.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rank[out[j].ID] < rank[out[j-1].ID]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return append(out, unranked...)
}

// OutputVariable returns the name of the variable an action outputs, without
// `$`, or "" when it outputs none. A `declare` counts: the variable is what it
// produces, and `after $ValidFeedback` is how a reader would name it.
func OutputVariable(action microflows.MicroflowAction) string {
	switch a := action.(type) {
	case *microflows.CreateVariableAction:
		return a.VariableName
	case *microflows.CreateObjectAction:
		return a.OutputVariable
	case *microflows.RetrieveAction:
		return a.OutputVariable
	case *microflows.JavaActionCallAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	case *microflows.MicroflowCallAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	case *microflows.NanoflowCallAction:
		if a.UseReturnVariable {
			return a.OutputVariableName
		}
	case *microflows.JavaScriptActionCallAction:
		if a.UseReturnVariable {
			return a.OutputVariableName
		}
	case *microflows.AggregateListAction:
		return a.OutputVariable
	case *microflows.ListOperationAction:
		return a.OutputVariable
	case *microflows.RestCallAction:
		return a.OutputVariable
	case *microflows.ImportMappingCallAction:
		return a.OutputVariable
	case *microflows.ExportMappingCallAction:
		return a.OutputVariable
	case *microflows.CallExternalAction:
		if a.UseReturnVariable {
			return a.ResultVariableName
		}
	}
	return ""
}

// normalizeStatement folds a rendered statement onto one line: describe breaks
// long expressions across lines, and a handle has to fit in a comment.
func normalizeStatement(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(strings.TrimSuffix(s, ";"))
}

// statementForms returns the token lists a pattern is matched against: the
// printed statement, then each alternate.
func (c *Candidate) statementForms() [][]Token {
	if c.forms == nil {
		c.forms = [][]Token{}
		for _, s := range append([]string{c.Statement}, c.Alternates...) {
			if s == "" {
				continue
			}
			// A rendering that does not tokenise can still be addressed by
			// variable or caption; it simply never matches a pattern.
			if toks, err := Tokenize(s); err == nil {
				c.forms = append(c.forms, toks)
			}
		}
	}
	return c.forms
}

// matchesPattern reports whether any form of c's statement matches pattern.
func (c *Candidate) matchesPattern(pattern []Token) bool {
	for _, toks := range c.statementForms() {
		if globTokens(pattern, toks) {
			return true
		}
	}
	return false
}

// matches reports whether c is addressed by t, ignoring t's ordinal.
func (c *Candidate) matches(t Target) bool {
	switch t.Kind {
	case ByOutputVariable:
		return c.OutputVariable != "" && c.OutputVariable == t.Variable
	case ByCaption:
		return c.Caption != "" && c.Caption == t.Caption
	default:
		return c.matchesPattern(t.Pattern)
	}
}

// describeCandidate is how a candidate is listed in an error.
func describeCandidate(c Candidate) string {
	stmt := c.Statement
	if stmt == "" {
		stmt = "<" + strings.TrimPrefix(fmt.Sprintf("%T", c.Object), "*microflows.") + ">"
	}
	if c.Caption != "" {
		stmt = fmt.Sprintf("%s  (caption %s)", stmt, quote(c.Caption))
	}
	if c.Object != nil {
		p := c.Object.GetPosition()
		stmt = fmt.Sprintf("%s  at (%d, %d)", stmt, p.X, p.Y)
	}
	return stmt
}

// quote renders s as an MDL string literal.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// isIdentRune reports whether r continues a bare word.
func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
