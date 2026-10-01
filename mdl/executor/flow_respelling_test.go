// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#886: an equivalent respelling of a statement — a keyword in
// another case, a member list laid out over several lines — is the stored
// statement, in the statement diff as in the built comparison. Compared as
// written, mdl 1 refused it on every run ("the Loop … changes inside its
// body") and, outside a loop, re-spliced it on every run.
func TestDeclaredMatches_EquivalentRespellingIsTheSame(t *testing.T) {
	// The stored side is what describe prints: one line, lower case.
	stored := parseFlowBody(t, `  @position(400, 200)
  if $In != empty and $In != 'x' then
    @position(400, 300)
    change $In (Name = $In, Active = false);
  end if;`)
	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"upper-case AND", `  if $In != empty AND $In != 'x' then
    change $In (Name = $In, Active = false);
  end if;`, true},
		{"member list over several lines", `  if $In != empty and $In != 'x' then
    change $In (
      Name = $In,
      Active = false
    );
  end if;`, true},
		{"both", `  if $In != empty
     AND $In != 'x' then
    change $In (
      Name = $In,
      Active = FALSE
    );
  end if;`, true},
		// Controls: a real change is still one.
		{"control: and becomes or", `  if $In != empty or $In != 'x' then
    change $In (Name = $In, Active = false);
  end if;`, false},
		{"control: a changed value over several lines", `  if $In != empty and $In != 'x' then
    change $In (
      Name = $In,
      Active = true
    );
  end if;`, false},
		{"control: case inside a string", `  if $In != empty AND $In != 'X' then
    change $In (Name = $In, Active = false);
  end if;`, false},
	} {
		declared := parseFlowBody(t, c.body)
		if got := declaredMatches(declared.Body, stored.Body); got != c.want {
			t.Errorf("%s: declaredMatches = %v, want %v", c.name, got, c.want)
		}
	}
}

// A flow stores one integer type, Integer/Long: `declare $N Long` is written
// as the Integer describe prints. Compared as written, the declare was
// replaced on every run of the formula1 script that drifted (D1).
func TestDeclaredMatches_LongIsTheStoredInteger(t *testing.T) {
	stored := parseFlowBody(t, `  @position(400, 200)
  declare $N Integer = 0;`)
	for _, c := range []struct {
		body string
		want bool
	}{
		{`  declare $N Long = 0;`, true},
		{`  declare $N Integer = 0;`, true},
		{`  declare $N Decimal = 0;`, false}, // control
		{`  declare $N Long = 1;`, false},    // control
	} {
		if got := declaredMatches(parseFlowBody(t, c.body).Body, stored.Body); got != c.want {
			t.Errorf("%s: declaredMatches = %v, want %v", c.body, got, c.want)
		}
	}
}

// The built graph of a member list laid out over several lines holds the line
// break in the last value, as written; the stored one does not. The same
// expression either way.
func TestSameBuiltFlow_ExpressionWhitespaceAndCaseAreNotADifference(t *testing.T) {
	for _, c := range []struct {
		name          string
		edit          func(oc *microflows.MicroflowObjectCollection, v string)
		built, stored string
		same          bool
	}{
		{"trailing line break in a value", setReturn, "$N\n      ", "$N", true},
		{"upper-case keyword in a condition", setCondition, "$N < 0 OR\n $N > 9", "$N < 0 or $N > 9", true},
		{"control: another value", setReturn, "$N + 1", "$N", false},
		{"control: another operator", setCondition, "$N < 0 AND $N > 9", "$N < 0 or $N > 9", false},
	} {
		built, stored := guardGraph("b-"), guardGraph("s-")
		c.edit(built, c.built)
		c.edit(stored, c.stored)
		if same, why := sameBuiltFlow(built, stored); same != c.same {
			t.Errorf("%s: same = %v (%s), want %v", c.name, same, why, c.same)
		}
	}
}

func setReturn(oc *microflows.MicroflowObjectCollection, v string) {
	oc.Objects[3].(*microflows.EndEvent).ReturnValue = v
}

func setCondition(oc *microflows.MicroflowObjectCollection, v string) {
	oc.Objects[1].(*microflows.ExclusiveSplit).SplitCondition.(*microflows.ExpressionSplitCondition).Expression = v
}
