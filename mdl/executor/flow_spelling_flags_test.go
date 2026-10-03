// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#942: an AST flag that records how a statement was SPELLED, not
// what it stores, is no difference to the statement diff. Describe prints one
// spelling, so comparing such a flag made a statement never match its own
// stored activity: re-spliced on every run, and next to a loop the run of
// changes swallowed the loop and rebuilt it with new $IDs under mdl 1.
func TestDeclaredMatches_SpellingOnlyFlagsAreNotADifference(t *testing.T) {
	for _, c := range []struct {
		name, stored, declared string
		want                   bool
	}{
		{"commit: with events is the stored bare commit",
			`  commit $In;`, `  commit $In with events;`, true},
		{"commit: bare", `  commit $In;`, `  commit $In;`, true},
		{"control: commit without events is not the bare commit",
			`  commit $In;`, `  commit $In without events;`, false},
		{"control: with events is not a stored without events",
			`  commit $In without events;`, `  commit $In with events;`, false},
		{"control: refresh added", `  commit $In;`, `  commit $In with events refresh;`, false},
		{"split type: legacy case/else is the when form describe prints",
			`  split type $In
    when M.A then
      log info node 'N' 'a';
    when (empty) then
      log info node 'N' 'e';
  end split;`,
			`  split type $In
    case M.A
      log info node 'N' 'a';
    else
      log info node 'N' 'e';
  end split;`, true},
		{"control: split type with a branch changed",
			`  split type $In
    when M.A then
      log info node 'N' 'a';
    when (empty) then
      log info node 'N' 'e';
  end split;`,
			`  split type $In
    case M.A
      log info node 'N' 'b';
    else
      log info node 'N' 'e';
  end split;`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			declared := parseFlowBody(t, c.declared)
			stored := parseFlowBody(t, c.stored)
			if got := declaredMatches(declared.Body, stored.Body); got != c.want {
				t.Errorf("declaredMatches = %v, want %v", got, c.want)
			}
		})
	}
}

// An `else` with nothing in it builds the same split as no else: the false
// flow goes on to what follows. Describe drops the empty else, so the declared
// form must be canonicalised to match its own stored `if`.
func TestCanonicalFlow_EmptyElseIsNoElse(t *testing.T) {
	stored := parseFlowBody(t, `  if $In = 'x' then
    log info node 'N' 'x';
  end if;
  log info node 'N' 'after';`)
	for _, c := range []struct {
		name, declared string
		want           bool
	}{
		{"empty else", `  if $In = 'x' then
    log info node 'N' 'x';
  else
  end if;
  log info node 'N' 'after';`, true},
		{"control: an else with a body", `  if $In = 'x' then
    log info node 'N' 'x';
  else
    log info node 'N' 'y';
  end if;
  log info node 'N' 'after';`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := canonicalFlow(parseFlowBody(t, c.declared).Body)
			s := canonicalFlow(stored.Body)
			if got := declaredMatches(d, s); got != c.want {
				t.Errorf("declaredMatches = %v, want %v", got, c.want)
			}
		})
	}
}
