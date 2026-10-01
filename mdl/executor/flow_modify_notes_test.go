// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#859: describe never prints `on error rollback` — it is what an
// activity with no clause stores — so a statement stating it must match the
// stored statement without it. Any other clause is still a difference (the
// controls), and so is a custom handler that happens to roll back.
func TestDeclaredMatches_OnErrorRollbackIsTheDescribedDefault(t *testing.T) {
	stored := parseFlowBody(t, `  commit $In;`)
	for body, want := range map[string]bool{
		`  commit $In on error rollback;`: true,
		`  commit $In;`:                   true,
		`  commit $In on error continue;`: false,
		"  commit $In on error begin\n    log info node 'N' 'x';\n  end error;": false,
	} {
		if got := declaredMatches(parseFlowBody(t, body).Body, stored.Body); got != want {
			t.Errorf("%q: declaredMatches = %v, want %v", body, got, want)
		}
	}
}

// ako/mxcli#859 (rehearsal M3): a replaced statement whose notes differ from
// the stored activity's keeps its own notes, for the builder to draw, and asks
// for the stored ones to go; one with the stored notes has them taken off (the
// splice keeps the stored ones). A note shared with another activity (it has
// an id) cannot be changed from one of them.
func TestKeepStoredNotes(t *testing.T) {
	stmt := func(body string) ast.MicroflowStatement {
		t.Helper()
		return parseFlowBody(t, body).Body[0]
	}
	notes := func(st ast.MicroflowStatement) int {
		if ann := statementAnnotations(st); ann != nil {
			return len(ann.Notes)
		}
		return 0
	}
	stored := stmt("  @annotation 'Old.'\n  log info node 'N' 'x';")
	cases := []struct {
		name, body  string
		wantReplace bool
		wantNotes   int
	}{
		{"the same note", "  @annotation 'Old.'\n  log info node 'N' 'y';", false, 0},
		{"reworded", "  @annotation 'New.'\n  log info node 'N' 'y';", true, 1},
		{"one added", "  @annotation 'Old.'\n  @annotation 'Added.'\n  log info node 'N' 'y';", true, 2},
		{"taken off", "  log info node 'N' 'y';", true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, replace, err := keepStoredNotes(stored, []ast.MicroflowStatement{stmt(c.body)})
			if err != nil {
				t.Fatalf("keepStoredNotes: %v", err)
			}
			if replace != c.wantReplace || notes(out[0]) != c.wantNotes {
				t.Errorf("replace %v with %d notes, want %v with %d", replace, notes(out[0]), c.wantReplace, c.wantNotes)
			}
		})
	}
	shared := stmt("  @annotation(id: n1, text: 'Shared.')\n  log info node 'N' 'x';")
	_, _, err := keepStoredNotes(shared, []ast.MicroflowStatement{stmt("  @annotation 'Mine.'\n  log info node 'N' 'x';")})
	var why *notSpliceable
	if !errors.As(err, &why) || !strings.Contains(err.Error(), "shared") {
		t.Errorf("re-annotating an activity whose note is shared: got %v, want a splice refusal", err)
	}
}
