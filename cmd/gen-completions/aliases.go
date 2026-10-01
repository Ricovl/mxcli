// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Completion offers the canonical language only (ako/mxcli#714 decision 5). A
// keyword token the parser grammar accepts nowhere except as a deprecated alias
// — `show_page`, `delete_behavior`, `define` — is a spelling nobody should be
// offered: under `mdl 1;` it warns, and from the version that removes it, it is
// refused.
//
// The grammar already says which these are. Every alias is marked where it is
// accepted, `SHOW_PAGE /* @alias MDL-DEPR020 */` (mdl/deprecation), so a token
// is alias-only when every use of it in a parser rule carries the marker. The
// `keyword` rule does not count: it lists the tokens that may also be written
// as a name, which says nothing about the token as a keyword. A token with no
// use at all is left alone; that is a different question from deprecation.
//
// A lexer rule whose first alternative is an alias needs nothing here: the
// generator takes the first alternative as the spelling, and each multi-spelling
// rule writes its canonical spelling first (MDLLexer.g4, R8).

var (
	blockCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentRE  = regexp.MustCompile(`//[^\n]*`)
	quotedRE       = regexp.MustCompile(`'(?:\\.|[^'\\])*'`)
	ruleHeadRE     = regexp.MustCompile(`(?s)^\s*([a-z][A-Za-z0-9_]*)\s*:(.*)$`)
	tokenRefRE     = regexp.MustCompile(`\b[A-Z][A-Z0-9_]*\b`)
	directiveRE    = regexp.MustCompile(`@[A-Za-z_]+(::[A-Za-z_]+)?`)
	// optionsRE is the word before an options/tokens/channels block, whose
	// braces stripBraces removes; the word would otherwise start the next
	// rule's chunk and hide its name.
	optionsRE = regexp.MustCompile(`\b(options|tokens|channels)\b`)
)

// aliasMarker replaces an `@alias` comment, so that stripping the other
// comments keeps it next to the token it marks. It is lower-case, so it is
// never read as a token reference, and has no `@`, so it survives the removal
// of the grammar's `@parser::members` directives.
const aliasMarker = " aliasmarker "

// parserGrammarSources lists the parser grammar beside the lexer: MDLParser.g4
// and the domain grammars it imports.
func parserGrammarSources(lexerPath string) ([]string, error) {
	dir := filepath.Dir(lexerPath)
	files := []string{filepath.Join(dir, "MDLParser.g4")}
	domains, err := filepath.Glob(filepath.Join(dir, "domains", "*.g4"))
	if err != nil {
		return nil, err
	}
	sort.Strings(domains)
	files = append(files, domains...)
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		out = append(out, string(b))
	}
	return out, nil
}

// aliasOnlyTokens returns the tokens every parser-rule use of which is marked
// as a deprecated alias.
func aliasOnlyTokens(sources []string) map[string]bool {
	canonical := map[string]bool{}
	aliased := map[string]bool{}
	for _, src := range sources {
		for name, body := range parserRules(src) {
			if name == "keyword" {
				continue
			}
			for _, loc := range tokenRefRE.FindAllStringIndex(body, -1) {
				tok := body[loc[0]:loc[1]]
				if strings.HasPrefix(strings.TrimLeft(body[loc[1]:], " \t\r\n"), strings.TrimSpace(aliasMarker)) {
					aliased[tok] = true
				} else {
					canonical[tok] = true
				}
			}
		}
	}
	out := map[string]bool{}
	for tok := range aliased {
		if !canonical[tok] {
			out[tok] = true
		}
	}
	return out
}

// parserRules splits a parser grammar into its rules, name to body, with
// comments (all but the alias markers), literals, actions and options removed.
func parserRules(src string) map[string]string {
	src = blockCommentRE.ReplaceAllStringFunc(src, func(c string) string {
		if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(c, "/*")), "@alias") {
			return aliasMarker
		}
		return " "
	})
	src = lineCommentRE.ReplaceAllString(src, " ")
	src = quotedRE.ReplaceAllString(src, " ")
	src = stripBraces(src)
	src = directiveRE.ReplaceAllString(src, " ")
	src = optionsRE.ReplaceAllString(src, " ")
	rules := map[string]string{}
	for chunk := range strings.SplitSeq(src, ";") {
		if m := ruleHeadRE.FindStringSubmatch(chunk); m != nil {
			rules[m[1]] += " " + m[2]
		}
	}
	return rules
}

// stripBraces removes every `{ … }` block: options, @members and actions.
func stripBraces(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '{':
			depth++
		case r == '}' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}
