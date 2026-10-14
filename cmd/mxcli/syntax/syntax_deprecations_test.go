// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// TestSyntaxShowsNoDeprecatedSpelling holds the Syntax field — metasyntax, which
// does not parse — to the deprecation registry.
//
// TestExamplesParse runs every Example through the validator, but the Syntax
// text is the part an agent reads as the grammar, and it is where the spellings
// `check` warns on survived: `NOT NULL [ERROR 'msg']` (MDL-DEPR021) and a named
// data grid `COLUMN name (` (MDL-DEPR005).
//
// The patterns come from the registry, never from a hand list, so a new entry is
// covered the day it lands. Each entry's Example and CanonicalExample differ
// only where the spelling changed; the tokens of that difference, with one token
// of context, are the deprecated spelling's fingerprint. Identifiers and string
// literals become wildcards (a Syntax text writes `name`, `<name>`, `'msg'`
// where the example has a real one); lower-case words stay literal keywords.
//
// To keep false positives out: `--` comments are not matched (prose may name the
// old form in order to warn against it), a fingerprint must hold a keyword and
// at least two tokens, and an entry whose two examples differ in a span too wide
// to be one spelling yields no fingerprint at all rather than a vague one.
func TestSyntaxShowsNoDeprecatedSpelling(t *testing.T) {
	clean := cleanCorpus()
	var fps []fingerprint
	for _, e := range deprecation.All() {
		if fp, ok := deprecatedFingerprint(e, clean); ok {
			fps = append(fps, fp)
			t.Logf("%s fingerprint: %s", e.Code, fp)
		}
	}
	// The derivation must keep producing patterns; if a registry reshuffle
	// left it with almost nothing, this test would pass vacuously.
	if len(fps) < 20 {
		t.Fatalf("only %d registry entries yielded a fingerprint — the derivation has stopped working", len(fps))
	}
	for _, f := range syntax.All() {
		toks := lexMDL(stripComments(f.Syntax))
		for _, fp := range fps {
			if syntaxDeprecationExempt[f.Path][fp.code] != "" {
				continue
			}
			if at := fp.find(toks); at >= 0 {
				t.Errorf("syntax topic %q, Syntax field, shows %s's deprecated spelling %q (matched %q) — write `%s`",
					f.Path, fp.code, fp.old, joinTokens(toks[at:at+len(fp.pat)]), fp.canonical)
			}
		}
	}
}

// syntaxDeprecationExempt lists the topics whose Syntax legitimately writes a
// spelling that is deprecated elsewhere, each with the registry's own reason.
// Keep it to what the entry's Note names: an exemption stops the check for
// that code on the whole topic.
var syntaxDeprecationExempt = map[string]map[string]string{
	// MDL-DEPR001's Note: not reported for `create or replace translations`,
	// which replaces the whole set — a different meaning, not an alias.
	"translations": {deprecation.CreateOrReplace: "create or replace translations is not an alias"},
}

// TestDeprecatedFingerprintsMatchTheirOwnExample pins the derivation itself:
// a fingerprint that does not even find its entry's Example would let every
// Syntax text through, and one that finds the CanonicalExample would flag the
// spelling the docs are told to write.
func TestDeprecatedFingerprintsMatchTheirOwnExample(t *testing.T) {
	for _, e := range deprecation.All() {
		fp, ok := deprecatedFingerprint(e, cleanCorpus())
		if !ok {
			continue
		}
		if fp.find(lexMDL(e.Example)) < 0 {
			t.Errorf("%s: fingerprint %s does not find the entry's own Example %q", e.Code, fp, e.Example)
		}
		if fp.find(lexMDL(e.CanonicalExample)) >= 0 {
			t.Errorf("%s: fingerprint %s finds the CanonicalExample %q", e.Code, fp, e.CanonicalExample)
		}
	}
}

// cleanCorpus is MDL known to carry no deprecated spelling: every entry's
// CanonicalExample, and every syntax topic's Example, which TestExamplesParse
// holds to `check --deprecations=error`. A fingerprint that matches any of it
// is too vague to tell the deprecated spelling from the current one.
// Comments are dropped: prose may quote the old form to warn against it.
func cleanCorpus() [][]token {
	var out [][]token
	for _, e := range deprecation.All() {
		out = append(out, lexMDL(e.CanonicalExample))
	}
	for _, f := range syntax.All() {
		if nonStatementTopics[f.Path] {
			continue // prose, not held to the validator
		}
		out = append(out, lexMDL(stripComments(f.Example)))
	}
	return out
}

type tokKind int

const (
	tkWord   tokKind = iota // keyword-shaped word: lower-case in the registry
	tkIdent                 // identifier, qualified name, $variable, <placeholder>
	tkString                // '…'
	tkNumber
	tkPunct
)

type token struct {
	kind tokKind
	text string // lower-cased
}

var lexRe = regexp.MustCompile(`'(?:[^']|'')*'|\$[A-Za-z_]\w*|<[^<>\s][^<>]*>|[A-Za-z_][\w]*(?:\.[A-Za-z_]\w*)*|\d+(?:\.\d+)?|…|\.\.\.|\S`)

// lexMDL splits MDL or metasyntax into tokens. Whether a word is a keyword or a
// name is not decidable from text alone; the registry's examples write
// keywords in lower case and names capitalised or qualified, and the
// fingerprint relies on that (see classify).
func lexMDL(s string) []token {
	var out []token
	for _, m := range lexRe.FindAllString(s, -1) {
		// Square brackets are dropped on both sides: in a Syntax text they
		// are the optional-group metasyntax, which splits a spelling apart
		// (`NOT NULL [ERROR 'msg']`) without changing it.
		if m == "[" || m == "]" {
			continue
		}
		out = append(out, classify(m))
	}
	return out
}

func classify(m string) token {
	low := strings.ToLower(m)
	switch {
	case strings.HasPrefix(m, "'"):
		return token{tkString, low}
	case strings.HasPrefix(m, "$"), strings.HasPrefix(m, "<") && len(m) > 1:
		return token{tkIdent, low}
	case m[0] >= '0' && m[0] <= '9':
		return token{tkNumber, low}
	case m[0] == '_' || (m[0]|0x20 >= 'a' && m[0]|0x20 <= 'z'):
		if m != low || strings.Contains(m, ".") {
			return token{tkIdent, low}
		}
		return token{tkWord, low}
	}
	return token{tkPunct, low}
}

func stripComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.Index(l, "--"); j >= 0 {
			lines[i] = l[:j]
		}
	}
	return strings.Join(lines, "\n")
}

type fingerprint struct {
	code, old, canonical string
	pat                  []token
}

func (fp fingerprint) String() string { return "`" + joinTokens(fp.pat) + "`" }

func joinTokens(ts []token) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.text
	}
	return strings.Join(parts, " ")
}

// maxHunk bounds the changed span an entry may have and still be called one
// spelling. A structural rewrite that reorders a whole clause differs over a
// wider span; matching that span verbatim would find nothing useful, and
// shrinking it would find too much.
const maxHunk = 3

// maxContext bounds how many context tokens a fingerprint may grow by while it
// is being made specific enough (see deprecatedFingerprint).
const maxContext = 4

// deprecatedFingerprint derives the token pattern of e's deprecated spelling
// from the difference between its Example and CanonicalExample.
//
// The changed span alone is often a bare keyword (`snippet`, `remove`), so it
// starts with one token of context and then widens, a token at a time on
// alternating sides, until it is specific: it holds a keyword and matches no
// text in clean — MDL known to carry no deprecated spelling — and does not
// end on a name. A fingerprint that
// cannot get specific within maxContext tokens is dropped, never kept vague.
func deprecatedFingerprint(e deprecation.Entry, clean [][]token) (fingerprint, bool) {
	if e.Example == "" || e.CanonicalExample == "" {
		return fingerprint{}, false
	}
	old, can := lexMDL(e.Example), lexMDL(e.CanonicalExample)
	p := 0
	for p < len(old) && p < len(can) && old[p].text == can[p].text {
		p++
	}
	q := 0
	for q < len(old)-p && q < len(can)-p && old[len(old)-1-q].text == can[len(can)-1-q].text {
		q++
	}
	lo, hi := p, len(old)-q
	if hi-lo > maxHunk {
		return fingerprint{}, false
	}
	hasWildcard := false
	for _, t := range old[lo:hi] {
		if t.kind == tkIdent || t.kind == tkString || t.kind == tkNumber {
			hasWildcard = true
		}
	}
	// An insertion, or a name/literal deprecated in place, says nothing until
	// both neighbours say where. A changed keyword needs only one.
	if lo == hi || hasWildcard {
		if lo == 0 || hi == len(old) {
			return fingerprint{}, false
		}
		lo, hi = lo-1, hi+1
	} else if lo > 0 {
		lo--
	} else if hi < len(old) {
		hi++
	}
	canonical := [][]token{can}
	specific := func(pat []token) bool {
		keyword := false
		for _, t := range pat {
			if t.kind == tkWord {
				keyword = true
			}
		}
		if !keyword || len(pat) < 2 {
			return false
		}
		// A name at the edge is a wildcard that any prose word satisfies
		// (`Ready comment` would match "a comment"): anchor past it.
		if pat[0].kind == tkIdent || pat[len(pat)-1].kind == tkIdent {
			return false
		}
		probe := fingerprint{pat: pat}
		for _, c := range append(canonical, clean...) {
			if probe.find(c) >= 0 {
				return false
			}
		}
		return true
	}
	widen, preferPrev := 0, true
	for !specific(old[lo:hi]) {
		if widen == maxContext {
			return fingerprint{}, false
		}
		switch {
		case preferPrev && lo > 0, hi == len(old) && lo > 0:
			lo--
		case hi < len(old):
			hi++
		default:
			return fingerprint{}, false
		}
		widen++
		preferPrev = !preferPrev
	}
	return fingerprint{code: e.Code, old: e.Old, canonical: e.Canonical, pat: old[lo:hi]}, true
}

// find returns the index in ts where the fingerprint matches, or -1. A
// keyword or punctuation token matches itself (case-insensitively); an
// identifier matches any name-shaped token, a string any string, a number a
// number or a name (metasyntax writes `n`).
func (fp fingerprint) find(ts []token) int {
	for i := 0; i+len(fp.pat) <= len(ts); i++ {
		ok := true
		for j, want := range fp.pat {
			if !tokenMatches(want, ts[i+j]) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func tokenMatches(want, got token) bool {
	switch want.kind {
	case tkIdent:
		return got.kind == tkIdent || got.kind == tkWord
	case tkString:
		return got.kind == tkString
	case tkNumber:
		return got.kind == tkNumber || got.kind == tkWord || got.kind == tkIdent
	default:
		return (got.kind == tkWord || got.kind == tkIdent || got.kind == tkPunct) && got.text == want.text
	}
}
