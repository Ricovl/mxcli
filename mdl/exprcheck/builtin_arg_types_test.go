// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// builtinProbeVars are the parameters every probe microflow declared when the
// cases below were measured: ($d: Decimal, $i: Integer, $l: Long, $s: String,
// $b: Boolean, $dt: DateTime).
var builtinProbeVars = map[string]TypeKind{
	"d": KindDecimal, "i": KindInteger, "l": KindLong,
	"s": KindString, "b": KindBoolean, "dt": KindDateTime,
}

// TestBuiltinCallHints_MeasuredArgumentTypes pins E005 (a built-in given an
// argument of the wrong kind) and E006 (wrong argument count) to what mxbuild
// does. Every row was built as `return <expr>;` in its own microflow with the
// parameters above and run through `mx check` on 11.6.6 AND 11.15.0, which
// agreed on every row: wantCode "" built at 0 errors, every other row is
// CE0117 "Error(s) in expression" (mendixlabs/mxcli#1515).
func TestBuiltinCallHints_MeasuredArgumentTypes(t *testing.T) {
	cases := []struct {
		expr     string
		wantCode string
	}{
		// The reported case: a number where the String pattern goes.
		{"formatDecimal($d, 2)", "E005"},
		{"formatDecimal($d, '#,##0.00')", ""},
		{"formatDecimal($i, '#')", ""}, // Integer widens to Decimal
		{"formatDecimal($l, '#')", ""}, // so does Long
		{"formatDecimal($d, $s)", ""},
		{"formatDecimal($d, '#', 'en-US')", ""},
		{"formatDecimal($d, '#', 1)", "E005"},
		{"formatDecimal($d, '#', $i)", "E005"},
		// The pattern is not optional, whatever the documentation suggests.
		{"formatDecimal($d)", "E006"},
		{"formatDecimal($d, '#', 'en', 4)", "E006"},

		// number → String
		{"length(5)", "E005"},
		{"length($i)", "E005"},
		{"toUpperCase($d)", "E005"},
		{"contains($s, 1)", "E005"},
		{"isMatch($s, 5)", "E005"},
		{"replaceAll($s, 'a', 1)", "E005"},
		{"replaceAll($s, $i, 'a')", "E005"},
		{"urlEncode(1)", "E005"},
		{"parseInteger(5)", "E005"},
		{"parseDateTime(5, 'yyyy')", "E005"},
		{"formatDate($dt, 5)", "E005"},
		{"trim(if $b then 1 else 2)", "E005"},
		// String + number concatenates: still a String.
		{"length($s + $i)", ""},
		{"startsWith($s, 'a' + 1)", ""},
		{"trim(if $b then 'a' else 'b')", ""},

		// Boolean / DateTime → String
		{"length(true)", "E005"},
		{"trim($b)", "E005"},
		{"length($dt)", "E005"},
		{"urlDecode($dt)", "E005"},

		// String / Boolean / DateTime → number
		{"substring('abc', '1')", "E005"},
		{"addDays($dt, '1')", "E005"},
		{"round('1.5')", "E005"},
		{"dateTime('2020', 1, 1)", "E005"},
		{"parseInteger($s, 'x')", "E005"},
		{"pow($s, 2)", "E005"},
		{"floor($s)", "E005"},
		{"round(true)", "E005"},
		{"sqrt($b)", "E005"},
		{"addDays($dt, $b)", "E005"},
		{"abs($dt)", "E005"},
		{"addDays($dt, $dt)", "E005"},
		{"min($d, 'x')", "E005"},
		{"max($b, $b)", "E005"},

		// Decimal → Integer/Long is refused; every other numeric pairing passes.
		{"addDays($dt, 1.5)", "E005"},
		{"addDays($dt, $d)", "E005"},
		{"subtractDays($dt, 1.5)", "E005"},
		{"addMonthsUTC($dt, 1.5)", "E005"},
		{"substring('abc', $d)", "E005"},
		{"parseInteger($s, 1.5)", "E005"},
		{"round($d, $d)", "E005"},
		{"dateTime(2020.5, 1, 1)", "E005"},
		{"epochToDateTime($d)", "E005"},
		{"addDays($dt, $l)", ""},
		{"addSeconds($dt, $l)", ""},
		{"substring($s, $l)", ""},
		{"round($d, $l)", ""},
		{"parseInteger($s, $l)", ""},
		{"epochToDateTime($i)", ""},
		{"epochToDateTime($l)", ""},
		{"abs($l)", ""},
		{"floor($i)", ""},
		{"sqrt($i)", ""},
		{"pow($i, $l)", ""},

		// String / number → DateTime
		{"addDays('2020-01-01', 1)", "E005"},
		{"formatDateTime($s, 'yyyy')", "E005"},
		{"dateTimeToEpoch($s)", "E005"},
		{"millisecondsBetween($s, $dt)", "E005"},
		{"daysBetween($dt, $s)", "E005"},
		{"parseDateTime($s, 'yyyy', 'x')", "E005"},
		{"parseDateTime($s, 'yyyy', $d)", "E005"},
		{"beginOfDay(5)", "E005"},

		// parseDecimal's second argument is a format OR a default.
		{"parseDecimal($s, 2)", ""},
		{"parseDecimal($s, $d)", ""},
		{"parseDecimal($s, '#', $i)", ""},
		{"parseDecimal($s, $s, $l)", ""},
		{"parseDecimal($s, '#', 'x')", "E005"},

		// Arity rows the signature table had wrong. Each was E006 on the
		// project tier; each builds at 0 errors.
		{"find($s, 'a', 2)", ""},
		{"findLast($s, 'a', 2)", ""},
		{"find($s, 'a', 'b')", "E005"},
		{"max($i, $i, $i)", ""},
		{"min(1, 2, 3)", ""},
		{"max($i, 1, 2, 3, 4)", ""},
		{"min(1, 2.5, $l)", ""},
		{"max($dt, $dt)", ""}, // max/min take DateTime too
		{"min($dt, $dt)", ""},
		{"dateTime(2020)", ""},
		{"dateTime(2020, 1)", ""},
		{"dateTime(2020, 1, 1, 10)", ""},
		{"dateTime(2020, 1, 1, 10, 30)", ""},
		{"dateTimeUTC(2020)", ""},
		{"formatDateTime($dt)", ""},
		{"formatDateTimeUTC($dt)", ""},
		{"formatTime($dt)", ""},
		{"formatDateUTC($dt)", ""},
		{"formatDateTime($dt, 'yyyy', 'en-US')", "E006"},
		{"parseDateTime($s)", "E006"},
		{"substring('abc', 1, 2, 3)", "E006"},
		{"length($s, 2)", "E006"},
		{"random(1)", "E006"},
		{"toString()", "E006"},

		// Controls the check must not touch.
		{"toString($dt)", ""},
		{"parseDateTimeUTC($s, 'yyyy', $dt)", ""},
		{"max($d, $i)", ""},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			hs := BuiltinCallHints(c.expr, builtinProbeVars)
			got := ""
			if len(hs) > 0 {
				got = hs[0].Code
			}
			if got != c.wantCode {
				t.Errorf("BuiltinCallHints(%q) = %q, want %q (hints: %+v)", c.expr, got, c.wantCode, hs)
			}
		})
	}
}

// TestBuiltinCallHints_UnknownKindsAreTolerated keeps the check to what it can
// prove: an argument whose kind is not known — an undeclared variable, an
// attribute path without a project, an enumeration, a token — is never an
// error. `empty` is tolerated too even though mxbuild refuses `length(empty)`:
// it is a separate question from a kind mismatch and not this rule's claim.
func TestBuiltinCallHints_UnknownKindsAreTolerated(t *testing.T) {
	for _, expr := range []string{
		"formatDecimal($Unknown, 2.0)", // arg 2 is still a number: flagged below
		"length($Unknown)",
		"length($Order/Name)",
		"addDays([%CurrentDateTime%], 1)",
		"formatDateTime([%BeginOfCurrentDay%], 'yyyy')",
		"length(empty)",
		"contains($e, 'x')",
		"getCaption($s)",
	} {
		hs := BuiltinCallHints(expr, map[string]TypeKind{"s": KindString, "e": KindEnumeration})
		if expr == "formatDecimal($Unknown, 2.0)" {
			if len(hs) != 1 || hs[0].Code != "E005" {
				t.Errorf("%s: want exactly the E005 on the pattern argument, got %+v", expr, hs)
			}
			continue
		}
		if len(hs) != 0 {
			t.Errorf("%s: want no hint, got %+v", expr, hs)
		}
	}
}

// TestBuiltinCallHints_NestedCallTyped checks that a call's result kind feeds
// the enclosing call: toString(minutesBetween(...)) is fine, while
// length(minutesBetween(...)) hands a Decimal to a String parameter.
func TestBuiltinCallHints_NestedCallTyped(t *testing.T) {
	if hs := BuiltinCallHints("toString(minutesBetween($a, $b))", map[string]TypeKind{"a": KindDateTime, "b": KindDateTime}); len(hs) != 0 {
		t.Errorf("toString of a Decimal must pass, got %+v", hs)
	}
	hs := BuiltinCallHints("length(minutesBetween($a, $b))", map[string]TypeKind{"a": KindDateTime, "b": KindDateTime})
	if len(hs) != 1 || hs[0].Code != "E005" {
		t.Errorf("length(<Decimal>) must be E005, got %+v", hs)
	}
}
