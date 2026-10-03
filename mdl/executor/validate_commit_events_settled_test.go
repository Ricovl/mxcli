// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// storedCommitFlows answers CommitEvents from a fixed table: flow name ->
// variable -> the WithEvents flag of each stored Commit activity.
type storedCommitFlows map[string]map[string][]bool

func (s storedCommitFlows) CommitEvents(_ bool, qn string) (map[string][]bool, bool) {
	ev, ok := s[qn]
	return ev, ok
}

// TestDropSettledCommitNotes pins when exec stays quiet about MDL067: only when
// the stored flow already commits every variable the script commits bare
// exactly as the script will write it. A re-run of an idempotent script then
// changes nothing about events, and the note — "this statement now writes the
// opposite of what it used to" — is false for it. Every other case still
// changes, or may change, what is stored, and keeps the note.
func TestDropSettledCommitNotes(t *testing.T) {
	const flow = "M.F"
	tests := []struct {
		name     string
		src      string
		stored   storedCommitFlows
		wantNote bool
	}{
		{
			name:     "stored with events, as the bare commit writes: settled",
			src:      "create or modify microflow M.F ($X: M.E) begin commit $X; end;",
			stored:   storedCommitFlows{flow: {"X": {true}}},
			wantNote: false,
		},
		{
			name: "explicit and bare commits of one variable all match: settled",
			src: "create or modify microflow M.F ($X: M.E) begin commit $X; " +
				"commit $X without events; commit $X; end;",
			stored:   storedCommitFlows{flow: {"X": {false, true, true}}},
			wantNote: false,
		},
		{
			name:     "stored without events — an older mxcli wrote it, the re-run flips it",
			src:      "create or modify microflow M.F ($X: M.E) begin commit $X; end;",
			stored:   storedCommitFlows{flow: {"X": {false}}},
			wantNote: true,
		},
		{
			name:     "the flow is not stored yet",
			src:      "create or modify microflow M.F ($X: M.E) begin commit $X; end;",
			stored:   storedCommitFlows{},
			wantNote: true,
		},
		{
			name:     "the script adds a bare commit the stored flow does not have",
			src:      "create or modify microflow M.F ($X: M.E) begin commit $X; commit $X; end;",
			stored:   storedCommitFlows{flow: {"X": {true}}},
			wantNote: true,
		},
		{
			name: "one variable settled, another not",
			src: "create or modify microflow M.F ($X: M.E, $Y: M.E) begin commit $X; " +
				"commit $Y; end;",
			stored:   storedCommitFlows{flow: {"X": {true}, "Y": {false}}},
			wantNote: true,
		},
		{
			name:     "a plain create is new code, whatever is stored",
			src:      "create microflow M.F ($X: M.E) begin commit $X; end;",
			stored:   storedCommitFlows{flow: {"X": {true}}},
			wantNote: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := visitor.Build(tt.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			before := ValidateProgram(prog, "")
			// The control: without a project the note always fires, so a
			// missing note below is the filter's doing, not the validator's.
			if countRule(before, "MDL067") != 1 {
				t.Fatalf("control: want one MDL067 before filtering, got %d", countRule(before, "MDL067"))
			}
			after := DropSettledCommitNotes(before, prog, tt.stored)
			if got := countRule(after, "MDL067") == 1; got != tt.wantNote {
				t.Errorf("MDL067 present = %v, want %v", got, tt.wantNote)
			}
			if len(before)-len(after) > 1 {
				t.Errorf("dropped %d violations; only the one MDL067 note may go", len(before)-len(after))
			}
		})
	}
}

// TestDropSettledCommitNotesWithoutProject: no stored flows to ask, nothing
// to drop.
func TestDropSettledCommitNotesWithoutProject(t *testing.T) {
	prog, errs := visitor.Build("create or modify microflow M.F ($X: M.E) begin commit $X; end;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	vs := ValidateProgram(prog, "")
	if got := DropSettledCommitNotes(vs, prog, nil); countRule(got, "MDL067") != 1 {
		t.Errorf("with no stored flows the note must stay")
	}
}

func countRule(vs []linter.Violation, id string) int {
	n := 0
	for _, v := range vs {
		if v.RuleID == id {
			n++
		}
	}
	return n
}
