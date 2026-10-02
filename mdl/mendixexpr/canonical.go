// SPDX-License-Identifier: Apache-2.0

package mendixexpr

import "strings"

// mendixKeywords are the words a Mendix expression reads whatever their case:
// the word operators and the literal keywords. A rebuilt expression always
// spells them lower case (String); preserved source text may not.
var mendixKeywords = map[string]bool{
	"and": true, "or": true, "not": true, "div": true, "mod": true,
	"true": true, "false": true, "empty": true,
	"if": true, "then": true, "else": true,
}

// Canonical is a Mendix expression in the one spelling of its tokens that
// every equivalent respelling shares (ako/mxcli#886), for deciding whether two
// expressions say the same thing — never for storing:
//
//   - whitespace and line breaks between tokens are not part of it: a run of
//     them is dropped, except a single space where it keeps two words apart
//     (`a and b`) or two operator characters (`< =` is not `<=`);
//   - a keyword (and, or, not, div, mod, true, false, empty, if, then, else)
//     reads the same in any case, so it is written lower case — unless it is
//     a name: a word after `.`, `/`, `$` or `@`, or before `.`, or a word
//     where its keyword cannot stand (keywordUse) — `Mod` in `find($L, Mod = 1)`
//     is a bare member name, and `Mod` and `mod` are two members
//     (ako/mxcli#898);
//   - a string literal is data, copied as written, a doubled apostrophe included.
//
// A declared statement whose member list a script lays out over several lines
// keeps the line break before `)` in the last value (the visitor preserves an
// expression slot's trailing whitespace), and preserved source keeps `AND`
// where describe prints `and`: compared as written, both were a change on
// every run.
func Canonical(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	space := false
	var last byte         // the last byte written outside a string, 0 at the start
	afterOperand := false // the last token ends an operand
	for i := 0; i < len(src); {
		c := src[i]
		switch c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			space = true
			i++
			continue
		}
		if space && last != 0 && (IsWordByte(last) && startsWord(c) || isOperatorByte(last) && isOperatorByte(c)) {
			b.WriteByte(' ')
		}
		space = false
		switch {
		case c == '\'':
			end := literalEnd(src, i)
			b.WriteString(src[i:end])
			last = '\''
			afterOperand = true
			i = end
		case IsWordByte(c):
			j := i
			for j < len(src) && IsWordByte(src[j]) {
				j++
			}
			word := src[i:j]
			prev := byte(0)
			if i > 0 {
				prev = src[i-1]
			}
			named := prev == '.' || prev == '/' || prev == '$' || prev == '@' || (j < len(src) && src[j] == '.')
			lw := strings.ToLower(word)
			keyword := !named && mendixKeywords[lw] && keywordUse(lw, afterOperand, src, j)
			if keyword {
				word = lw
			}
			afterOperand = !keyword || mendixLiterals[lw]
			b.WriteString(word)
			last = src[j-1]
			i = j
		default:
			b.WriteByte(c)
			last = c
			afterOperand = c == ')' || c == ']'
			i++
		}
	}
	return b.String()
}

// mendixLiterals are the keywords that are operands themselves.
var mendixLiterals = map[string]bool{"true": true, "false": true, "empty": true}

// keywordUse reports whether the word lw, which ends at src[j], is used as
// the keyword it is spelled like rather than as a bare member name, by where it
// stands (ako/mxcli#898). A binary operator (and, or, div, mod) and then/else
// follow an operand; not and if precede one and cannot follow one; anywhere
// else the word is a name — `Mod` in `find($L, Mod = 1)`, `Not` in `f(Not)`.
// afterOperand reports whether the token before the word ends an operand: a
// name, a literal, `)` or `]`. A literal keyword (true, false, empty) is one
// wherever it stands.
func keywordUse(lw string, afterOperand bool, src string, j int) bool {
	switch lw {
	case "and", "or", "div", "mod", "then", "else":
		return afterOperand
	case "not", "if":
		return !afterOperand && startsOperandAt(src, j)
	}
	return true
}

// startsOperandAt reports whether the first non-blank byte at or after j can
// begin an operand.
func startsOperandAt(src string, j int) bool {
	for ; j < len(src); j++ {
		switch c := src[j]; c {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			continue
		default:
			return startsWord(c) || c == '(' || c == '\'' || c == '-' || c == '['
		}
	}
	return false
}

// startsWord reports whether c begins a word or a name: a variable's `$` and a
// constant's `@` are part of the name they start.
func startsWord(c byte) bool { return IsWordByte(c) || c == '$' || c == '@' }

func isOperatorByte(c byte) bool { return strings.IndexByte("<>=!+-*/:", c) >= 0 }

// literalEnd returns the index just past the Mendix string literal that opens
// at start: a doubled apostrophe is one inside it, the only escape. An unterminated
// literal runs to the end.
func literalEnd(s string, start int) int {
	for i := start + 1; i < len(s); i++ {
		if s[i] != '\'' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '\'' {
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}
