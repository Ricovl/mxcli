// SPDX-License-Identifier: Apache-2.0

package mendixexpr

import "testing"

// ako/mxcli#886: two spellings of one expression — a keyword's case, the
// whitespace and line breaks between tokens — are one expression, and
// Canonical says so; anything else stays a difference.
func TestCanonical_EquivalentSpellingsAreEqual(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"$U/Name != empty AND $U/Name != 'x'", "$U/Name != empty and $U/Name != 'x'"},
		{"$A Or NOT($B)", "$A or not($B)"},
		{"$N Div 2 MOD 3", "$N div 2 mod 3"},
		{"TRUE", "true"},
		{"EMPTY", "empty"},
		{"IF $A THEN 1 ELSE 2", "if $A then 1 else 2"},
		{"false\n      ", "false"},
		{"  $A + $B  ", "$A + $B"},
		{"$Json + $Sep\n        + '{\"i\":' + toString($P/seq)", "$Json + $Sep + '{\"i\":' + toString($P/seq)"},
		{"$A+$B", "$A + $B"},
		{"not ( $A )", "not($A)"},
		{"f( $a ,\n $b )", "f($a, $b)"},
	} {
		if Canonical(c.a) != Canonical(c.b) {
			t.Errorf("%q and %q: canonical %q vs %q", c.a, c.b, Canonical(c.a), Canonical(c.b))
		}
	}
}

// Controls: what an expression means is never folded away.
func TestCanonical_DifferencesStay(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"$A and $B", "$A or $B"},
		{"'a  b'", "'a b'"},              // whitespace inside a string is data
		{"'AND'", "'and'"},               // so is case
		{"'it''s  x'", "'it''s x'"},      // past an escaped apostrophe too
		{"$U/And", "$U/and"},             // a member, not an operator
		{"$And", "$and"},                 // a variable
		{"M.Enum.TRUE", "M.Enum.true"},   // an enumeration value
		{"$A < = $B", "$A <= $B"},        // two operators stay two
		{"$a and b", "$a andb"},          // words stay apart
		{"toString($A)", "ToString($A)"}, // a function name is not a keyword
		{"$A - -1", "$A - 1"},
		{"false", "true"},
	} {
		if Canonical(c.a) == Canonical(c.b) {
			t.Errorf("%q and %q compare equal (%q)", c.a, c.b, Canonical(c.a))
		}
	}
}
