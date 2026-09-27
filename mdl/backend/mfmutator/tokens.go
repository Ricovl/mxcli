// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenKind classifies a Token.
type TokenKind int

const (
	// TokWord is a bare or "quoted" identifier, keyword or number. Compared
	// case-insensitively, as MDL keywords are.
	TokWord TokenKind = iota
	// TokVariable is `$Name`. Compared exactly.
	TokVariable
	// TokString is a '…' literal; Text holds the unescaped content. Compared
	// exactly: a caption or log message differing in case is a different one.
	TokString
	// TokStar is `*`: a wildcard in a pattern, multiplication in a statement.
	TokStar
	// TokPunct is any other single character.
	TokPunct
)

// Token is one lexical unit of a statement or pattern.
type Token struct {
	Kind TokenKind
	Text string
}

// Tokenize splits MDL statement text into Tokens. It is deliberately coarser
// than the MDL lexer — multi-character operators come out as single
// characters — because it only has to agree with itself: a pattern and a
// statement are tokenised the same way, so `!=` in one matches `!=` in the
// other whatever the spacing.
func Tokenize(s string) ([]Token, error) {
	var out []Token
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
		case r == '\'':
			text, n, err := scanQuoted(s[i:], '\'')
			if err != nil {
				return nil, err
			}
			out = append(out, Token{Kind: TokString, Text: text})
			i += n
		case r == '"':
			text, n, err := scanQuoted(s[i:], '"')
			if err != nil {
				return nil, err
			}
			out = append(out, Token{Kind: TokWord, Text: text})
			i += n
		case r == '$':
			j := i + size
			for j < len(s) {
				r2, sz := utf8.DecodeRuneInString(s[j:])
				if !isIdentRune(r2) {
					break
				}
				j += sz
			}
			if j == i+size {
				out = append(out, Token{Kind: TokPunct, Text: "$"})
			} else {
				out = append(out, Token{Kind: TokVariable, Text: s[i:j]})
			}
			i = j
		case isIdentRune(r):
			j := i
			for j < len(s) {
				r2, sz := utf8.DecodeRuneInString(s[j:])
				if !isIdentRune(r2) {
					break
				}
				j += sz
			}
			out = append(out, Token{Kind: TokWord, Text: s[i:j]})
			i = j
		case r == '*':
			out = append(out, Token{Kind: TokStar, Text: "*"})
			i += size
		default:
			out = append(out, Token{Kind: TokPunct, Text: string(r)})
			i += size
		}
	}
	return out, nil
}

// scanQuoted reads a literal delimited by q, where a doubled q is an escaped
// one (MDL's only escape for both string literals and quoted identifiers). It
// returns the unescaped content and the number of bytes consumed.
func scanQuoted(s string, q byte) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] != q {
			b.WriteByte(s[i])
			continue
		}
		if i+1 < len(s) && s[i+1] == q {
			b.WriteByte(q)
			i++
			continue
		}
		return b.String(), i + 1, nil
	}
	return "", 0, fmt.Errorf("unterminated %c literal in %q", q, s)
}

// sameToken reports whether a pattern token matches a statement token.
func sameToken(p, s Token) bool {
	if p.Kind != s.Kind {
		return false
	}
	if p.Kind == TokWord {
		return strings.EqualFold(p.Text, s.Text)
	}
	return p.Text == s.Text
}

// globTokens reports whether pattern matches the whole of stmt, a pattern
// TokStar matching any run of statement tokens (including none).
//
// The match is anchored at both ends. `commit $Order` therefore does not match
// `commit $Order with events` — write `commit $Order *` — so that adding a
// word to a pattern can only ever narrow what it matches.
func globTokens(pattern, stmt []Token) bool {
	p, s := 0, 0
	star, mark := -1, 0
	for s < len(stmt) {
		switch {
		case p < len(pattern) && pattern[p].Kind == TokStar:
			star, mark = p, s
			p++
		case p < len(pattern) && sameToken(pattern[p], stmt[s]):
			p++
			s++
		case star >= 0:
			p = star + 1
			mark++
			s = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p].Kind == TokStar {
		p++
	}
	return p == len(pattern)
}
