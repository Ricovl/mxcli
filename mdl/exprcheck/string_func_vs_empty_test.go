// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#580 (comment, 2026-10-30): `if trim($Value) != empty` passed
// `mxcli check --references`, `exec` and `mx check`, and was always true — the
// blank value went down the "has text" branch and crashed later in the flow.
//
// The String built-ins below never return `empty`. Measured on the Mendix
// 10.18.5 runtime with `mxcli test --local`: for an input of `empty`, the empty
// string and blanks, each returned the empty string or text, never `empty`,
// while the control — the bare variable — did compare equal to `empty`. So
// comparing one of these results with `empty` is a constant condition. Only
// measured functions are listed; one is added when it has been measured, not
// assumed.
package exprcheck

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

func TestStringFuncComparedWithEmpty(t *testing.T) {
	ctx := Context{Microflow: "M.Probe", SlotPath: "Decision"}
	cases := []struct {
		src  string
		want bool // E015 expected
	}{
		// The reported form, and its mirror and `=` variant.
		{"trim($Value) != empty", true},
		{"empty != trim($Value)", true},
		{"trim($Value) = empty", true},
		// Every measured function.
		{"toLowerCase($Value) = empty", true},
		{"toUpperCase($Value) = empty", true},
		{"substring($Value, 0) = empty", true},
		{"replaceAll($Value, 'x', 'y') = empty", true},
		{"toString($N) != empty", true},
		// Parenthesised still counts.
		{"(trim($Value)) != empty", true},
		// Inside a larger condition.
		{"$Flag and trim($Value) != empty", true},

		// Controls: the correct forms, and an operand that CAN be empty.
		{"trim($Value) != ''", false},
		{"$Value != empty", false},
		{"$Value != empty and trim($Value) != ''", false},
		// The redundant guard idiom does what it means: the `''` comparison
		// decides it, so the dead `empty` half is not reported. It is in the
		// Studio Pro-authored integration fixture (FeedbackModule.VAL_Feedback),
		// which is how the first version of E015 was caught flagging it.
		{"trim($Value) != empty and trim($Value) != ''", false},
		{"trim($Value) = empty or trim($Value) = ''", false},
		{"$Flag and trim($Value) != empty and trim($Value) != ''", false},
		// ...but only when the `''` comparison is on the SAME call.
		{"trim($Value) != empty and trim($Other) != ''", true},
		{"trim($Value) != empty and toLowerCase($Value) != ''", true},
		// A guard in a different chain does not cover it.
		{"(trim($Value) != empty or $Flag) and trim($Value) != ''", true},
		// An unmeasured function is left alone.
		{"getKey($Value) = empty", false},
		// Ordering against empty is not this defect.
		{"length($Value) > 0", false},
	}
	for _, tc := range cases {
		_, hs := NewParser().Parse(tc.src, ctx)
		got := false
		for _, h := range hs {
			if h.Code == "E015" {
				got = true
				if h.Severity != hints.SeverityWarning {
					t.Errorf("%q: E015 severity %v, want warning", tc.src, h.Severity)
				}
			}
		}
		if got != tc.want {
			t.Errorf("%q: E015 reported = %v, want %v (hints: %+v)", tc.src, got, tc.want, hs)
		}
	}
}
