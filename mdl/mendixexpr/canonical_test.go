// SPDX-License-Identifier: Apache-2.0

package mendixexpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

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

// ako/mxcli#898: the whitespace the visitor keeps after a slot's last token is
// layout; the stored expression ends at the token. Whitespace inside the
// expression, and inside a string at its end, is kept.
func TestString_SourceTrailingWhitespaceIsNotStored(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"false\n      ", "false"},
		{"$A + $B \t\r\n", "$A + $B"},
		{"$A\n  + $B", "$A\n  + $B"},
		{"'x  '", "'x  '"},
	} {
		if got := String(&ast.SourceExpr{Source: c.src}); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// ako/mxcli#898: a bare member name spelled like a keyword is a name where the
// keyword cannot stand, and a name keeps its case — `Mod` and `mod` are two
// members. Where the keyword can stand, its case still folds.
func TestCanonical_BareMemberNamedLikeAKeywordKeepsItsCase(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"Mod = 1", "mod = 1"},
		{"Div = 1", "div = 1"},
		{"Not = true", "not = true"},
		{"And != empty", "and != empty"},
		{"Or", "or"},
		{"$A and Then", "$A and then"},
		{"f(Not)", "f(not)"},
		{"Active and Mod = 1", "Active and mod = 1"},
	} {
		if Canonical(c.a) == Canonical(c.b) {
			t.Errorf("%q and %q compare equal (%q): a member's case was folded", c.a, c.b, Canonical(c.a))
		}
	}
	// Controls: the keyword where it stands still folds.
	for _, c := range []struct{ a, b string }{
		{"Mod = 1 AND $X", "Mod = 1 and $X"},
		{"NOT(Mod = 1)", "not(Mod = 1)"},
		{"NOT $A", "not $A"},
		{"$N MOD 2 = 0", "$N mod 2 = 0"},
		{"IF Mod = 1 THEN 'a' ELSE 'b'", "if Mod = 1 then 'a' else 'b'"},
		{"'x' OR $B", "'x' or $B"},
		{"f($a) DIV 2", "f($a) div 2"},
	} {
		if Canonical(c.a) != Canonical(c.b) {
			t.Errorf("%q and %q: canonical %q vs %q", c.a, c.b, Canonical(c.a), Canonical(c.b))
		}
	}
}

// The stored side agrees with Canonical: a bare member name keeps its case, an
// operator is lowered (mxcli-todo #14b).
func TestNormalizeOperatorCase_BareMemberNamedLikeAnOperator(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Mod = 1", "Mod = 1"},
		{"Active AND Mod = 1", "Active and Mod = 1"},
		{"NOT(Not)", "not(Not)"},
		{"$A != x AND $B != empty", "$A != x and $B != empty"},
		{"$N MOD 2 = 0 OR NOT $F", "$N mod 2 = 0 or not $F"},
		{"'AND' AND $x/Mod", "'AND' and $x/Mod"},
		{"M.Enum.And", "M.Enum.And"},
	} {
		if got := NormalizeOperatorCase(c.in); got != c.want {
			t.Errorf("NormalizeOperatorCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
