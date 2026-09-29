// SPDX-License-Identifier: Apache-2.0

// Package mendixexpr renders an MDL expression AST as the Mendix expression
// that is stored in the model. It is the executor's renderer for a
// re-rendered expression, in a package of its own so that the visitor can
// compute what a script stores without importing the executor: fmt --upgrade
// writes that stored form when a rewrite would otherwise change whether an
// expression is re-rendered or stored as written (ako/mxcli#804).
package mendixexpr

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixBuiltinFunctions is the canonical spelling of every built-in Mendix
// expression function. The expression runtime is case-sensitive: it only
// recognises these names as spelt here (lower-case with camelCase for
// compound words). Emitting an alternative spelling causes CE0117
// ("Error(s) in expression.") on Studio Pro validation.
//
// Source: https://docs.mendix.com/refguide/expressions/ and the linked
// function-specific pages (string, math, date arithmetic, parse/format,
// trim-to-date, list operations, aggregates, type conversions).
//
// The map key is the upper-case spelling for case-insensitive lookup; the
// value is the runtime-accepted canonical spelling. Custom user-defined
// java actions, sub-microflows, and unknown function names pass through
// unchanged so user case is preserved.
var mendixBuiltinFunctions = func() map[string]string {
	canonical := []string{
		// List operations
		"head", "tail", "find", "filter", "sort", "union",
		"intersect", "subtract", "contains", "equals", "range",
		// List aggregates
		"count", "sum", "average", "minimum", "maximum",
		"allTrue", "anyTrue",
		// String functions (docs.mendix.com/refguide/string-function-calls)
		"toUpperCase", "toLowerCase", "trim", "length", "substring",
		"findLast", "replaceAll", "replaceFirst", "startsWith", "endsWith",
		"isMatch", "isInvariantMatch", "stringFromRegex", "stringListFromRegex",
		"urlEncode", "urlDecode", "reverse", "indexOf",
		// Math functions (docs.mendix.com/refguide/mathematical-function-calls)
		"abs", "ceil", "floor", "round", "max", "min", "pow",
		"sqrt", "ln", "log10", "random", "rand",
		// Date creation (docs.mendix.com/refguide/date-creation)
		"dateTime", "dateTimeUTC",
		// Begin-of-date / end-of-date / trim-to-date
		"trimToDays", "trimToHours", "trimToMinutes", "trimToSeconds",
		"trimToDaysUTC", "trimToHoursUTC", "trimToMinutesUTC", "trimToSecondsUTC",
		"beginOfDay", "beginOfWeek", "beginOfMonth", "beginOfYear",
		"beginOfDayUTC", "beginOfWeekUTC", "beginOfMonthUTC", "beginOfYearUTC",
		"endOfDay", "endOfWeek", "endOfMonth", "endOfYear",
		"endOfDayUTC", "endOfWeekUTC", "endOfMonthUTC", "endOfYearUTC",
		// Between-date functions
		"millisecondsBetween", "secondsBetween", "minutesBetween",
		"hoursBetween", "daysBetween", "weeksBetween", "monthsBetween",
		"yearsBetween", "calendarDaysBetween", "calendarMonthsBetween",
		"calendarYearsBetween",
		// Add-date functions
		"addMilliseconds", "addSeconds", "addMinutes", "addHours",
		"addDays", "addWeeks", "addMonths", "addYears",
		"addDaysUTC", "addWeeksUTC", "addMonthsUTC", "addYearsUTC",
		// Subtract-date functions
		"subtractMilliseconds", "subtractSeconds", "subtractMinutes",
		"subtractHours", "subtractDays", "subtractWeeks", "subtractMonths",
		"subtractYears", "subtractDaysUTC", "subtractWeeksUTC",
		"subtractMonthsUTC", "subtractYearsUTC",
		// Day-of / timestamp conversion helpers
		"dayOfWeek", "dayOfWeekFromDateTime", "weekOfYearFromDateTime",
		"dayOfYearFromDateTime", "daysInMonth", "daysInYear",
		"dateTimeToEpoch", "epochToDateTime",
		// Parse / format (parse-and-format-date, parse-and-format-decimal)
		"formatDateTime", "formatDateTimeUTC", "parseDateTime", "parseDateTimeUTC",
		"parseInteger", "parseLong", "parseDecimal", "formatDecimal",
		// To-string / length  (to-string, length refguide pages)
		"toString", "toBoolean", "toFloat",
		// Enumeration helpers
		"getCaption", "getKey",
		// Miscellaneous
		"if", "empty", "isNew", "isAnonymous",
		// Boolean operators expressed as functions (true(), false())
		"true", "false",
		// Not / and / or appear as operators, not function calls — omitted.
	}
	m := make(map[string]string, len(canonical))
	for _, c := range canonical {
		m[strings.ToUpper(c)] = c
	}
	return m
}()

// FunctionName normalises the case of built-in Mendix expression
// functions. The visitor canonicalises list / aggregate operations in
// UPPERCASE for AST dispatch; the expression runtime only recognises the
// documented camelCase spelling. For every built-in Mendix function we
// always emit the canonical spelling so that:
//
//   - round-tripping a pristine microflow never mutates `find(...)` into
//     `FIND(...)` (which Studio Pro rejects with CE0117).
//   - LLM-generated MDL with accidental capitalisation (`LENGTH(...)`,
//     `ToString(...)`) still validates when executed.
//
// Custom (user-defined) java actions, sub-microflows and entity member
// references pass through unchanged so user case is preserved.
func FunctionName(name string) string {
	if canonical, ok := mendixBuiltinFunctions[strings.ToUpper(name)]; ok {
		return canonical
	}
	return name
}

// QuoteLiteral renders a Go string as a MENDIX expression literal.
//
// Its output goes into the stored document, so it must be what Mendix's
// expression engine reads — and that engine has exactly ONE escape: an
// apostrophe is doubled, SQL-style. There are no backslash escapes.
// Measured against a running 11.13 runtime: a microflow storing 'a\tb'
// (backslash, t) put FOUR bytes in the database, 61 5c 74 62 — a literal
// backslash and a 't', not a tab. That is the bug this function used to have.
//
// It escaped \n, \r and \t on the premise that "STRING_LITERAL does not accept
// them raw and the describe output has to survive check". The premise is false
// on both halves. The lexer rule is
//
//	STRING_LITERAL : '\'' ( ~['\\] | '\\' . | '\'\'' )* '\''
//
// and `~['\\]` admits every byte except an apostrophe and a backslash —
// newline, tab and carriage return included. A raw newline already round-tripped
// through describe → exec for exactly that reason, which is why only the tab
// looked broken: the newline path never reached the escape.
//
// So the escaping bought nothing and cost the value. A raw control character is
// now emitted as itself: correct in the document, and parseable on the way back.
//
// What IS still escaped, and must be:
//
//   - an apostrophe, doubled — the engine's only escape, and the MDL lexer's too;
//   - a backslash whose NEXT byte is one of n/r/t/\/', doubled — otherwise
//     unquoteString would decode the pair into a control character on reparse,
//     turning a literal two-character `\t` into a tab. A backslash before any
//     other byte passes through verbatim, so a regex literal like `^\d+$`
//     survives (the engine reads `\d` literally and hands it to the regex
//     compiler).
func QuoteLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\'':
			b.WriteString(`''`)
		case '\\':
			// Double the backslash only when the next byte would otherwise be
			// interpreted as an escape by unquoteString — that is, n/r/t/\/'.
			// For any other follower (letters like d/w, punctuation) the
			// backslash can pass through verbatim so regex escape characters
			// roundtrip without mutation.
			if i+1 < len(s) {
				switch s[i+1] {
				case 'n', 'r', 't':
					b.WriteString(`\\`)
					b.WriteByte(s[i+1])
					i++
					continue
				case '\\':
					// Literal backslash-backslash in AST. To survive roundtrip
					// it must be written as four backslashes: unquoteString
					// decodes `\\` twice, producing two backslashes again.
					b.WriteString(`\\\\`)
					i++
					continue
				case '\'':
					// Literal backslash-apostrophe: double the backslash and
					// double the apostrophe, so the reparsed value stays
					// [\, '].
					b.WriteString(`\\`)
					b.WriteString(`''`)
					i++
					continue
				}
				b.WriteByte('\\')
				continue
			}
			// Trailing backslash at end-of-string: the lexer's `'\\' .` escape
			// rule requires a following character, so emitting a bare `\'`
			// terminator would be reinterpreted as an escape pair and never
			// close the literal. Double the backslash — unquoteString decodes
			// `\\` back to a single backslash.
			b.WriteString(`\\`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// String converts an AST Expression to a Mendix expression string.
//
// The string it returns is STORED — `action.Expression = String(...)`
// — so it is Mendix's grammar throughout, not MDL's. Literals go through
// quoteExpressionLiteral, which doubles an apostrophe and otherwise emits the
// value as it is.
//
// This used to say the opposite, and reasoned from it: that escaping a control
// character was "the correct trade-off for describe→re-execute flows" because
// "emitting raw control chars in MDL would break the parser". Both halves were
// wrong. STRING_LITERAL's `~['\\]` accepts every byte but an apostrophe and a
// backslash, so raw control characters parse — and the trade-off was not one,
// because the escaped form is what the runtime then stores: 'a\tb' became the
// four bytes `a`, `\`, `t`, `b` in the database, not a tab.
func String(expr ast.Expression) string {
	// Check for nil interface
	if expr == nil {
		return ""
	}

	// Use reflection to check for nil pointer inside interface
	// This handles the Go interface gotcha where the type is set but pointer is nil
	if reflect.ValueOf(expr).IsNil() {
		return ""
	}

	switch e := expr.(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			return QuoteLiteral(fmt.Sprintf("%v", e.Value))
		case ast.LiteralBoolean:
			if e.Value.(bool) {
				return "true"
			}
			return "false"
		case ast.LiteralNull:
			return "empty"
		default:
			return fmt.Sprintf("%v", e.Value)
		}
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.AttributePathExpr:
		return "$" + e.Variable + "/" + strings.Join(e.Path, "/")
	case *ast.BinaryExpr:
		left := String(e.Left)
		right := String(e.Right)
		// Mendix expressions use lowercase operators (and, or, div, mod)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		// Mendix expressions use lowercase operators (not)
		op := strings.ToLower(e.Operator)
		// not(expr) — always emit parens; Mendix CE0117 rejects bare "not expr"
		if op == "not" {
			inner := e.Operand
			if paren, ok := inner.(*ast.ParenExpr); ok {
				// Unwrap double-parens: not((expr)) → not(expr)
				return "not(" + String(paren.Inner) + ")"
			}
			return "not(" + String(inner) + ")"
		}
		operand := String(e.Operand)
		return op + " " + operand
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, String(arg))
		}
		return FunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.TokenExpr:
		return "[%" + e.Token + "%]"
	case *ast.ParenExpr:
		return "(" + String(e.Inner) + ")"
	case *ast.IdentifierExpr:
		// Unquoted identifier (attribute name in XPath)
		return e.Name
	case *ast.QualifiedNameExpr:
		// Qualified name (association name, entity reference) - unquoted
		return e.QualifiedName.String()
	case *ast.ConstantRefExpr:
		return "@" + e.QualifiedName.String()
	case *ast.IfThenElseExpr:
		cond := String(e.Condition)
		thenStr := String(e.ThenExpr)
		elseStr := String(e.ElseExpr)
		return "if " + cond + " then " + thenStr + " else " + elseStr
	case *ast.SourceExpr:
		if e.Source != "" {
			return NormalizeOperatorCase(e.Source)
		}
		return String(e.Expression)
	default:
		return ""
	}
}

// mendixLowercaseOperators are the word operators Mendix requires in lowercase.
// A rebuilt BinaryExpr/UnaryExpr already gets this via strings.ToLower on the
// operator; preserved source text does not, which is the whole bug below.
var mendixLowercaseOperators = map[string]bool{
	"and": true, "or": true, "not": true, "div": true, "mod": true,
}

// NormalizeOperatorCase lowercases word operators in preserved expression
// source, leaving everything else — including string literals and member names —
// byte-identical.
//
// Some conditions are kept as a SourceExpr (original text plus the parsed tree)
// rather than rebuilt from the AST, and the raw branch skipped the lowercasing
// that a rebuilt expression gets. So `IF A != x AND B != empty` stored `AND`
// verbatim and the build failed with
//
//	[CE0117] "Error(s) in expression."
//
// while the same condition written with `=` was rebuilt as a BinaryExpr and
// normalised — which is why it looked like `!=` inside a conjunction was
// unsupported. It is the casing, not the operator. (mxcli-todo findings #14b)
//
// A word preceded by `.`, `/` or `$` is a member or variable name, never an
// operator, so `Module.Enum.And` and `$Task/Mod` are left alone.
func NormalizeOperatorCase(src string) string {
	var b strings.Builder
	b.Grow(len(src))

	inString := false
	for i := 0; i < len(src); {
		c := src[i]
		if inString {
			b.WriteByte(c)
			if c == '\'' {
				// '' is an escaped quote inside a Mendix string literal.
				if i+1 < len(src) && src[i+1] == '\'' {
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				inString = false
			}
			i++
			continue
		}
		if c == '\'' {
			inString = true
			b.WriteByte(c)
			i++
			continue
		}
		if !IsWordByte(c) {
			b.WriteByte(c)
			i++
			continue
		}
		j := i
		for j < len(src) && IsWordByte(src[j]) {
			j++
		}
		word := src[i:j]
		prev := byte(0)
		if i > 0 {
			prev = src[i-1]
		}
		if prev != '.' && prev != '/' && prev != '$' && mendixLowercaseOperators[strings.ToLower(word)] {
			b.WriteString(strings.ToLower(word))
		} else {
			b.WriteString(word)
		}
		i = j
	}
	return b.String()
}

// IsWordByte reports whether c can be part of a word operator or a name.
func IsWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
