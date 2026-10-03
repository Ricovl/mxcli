// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// commitEventSource is what DropSettledCommitNotes asks the project:
// StoredCommitEvents, or upgrade.StoredCommits by another name.
type commitEventSource interface {
	CommitEvents(nanoflow bool, qualifiedName string) (map[string][]bool, bool)
}

// DropSettledCommitNotes removes the MDL067 note (validate_commit_events.go)
// for each `create or modify microflow` whose stored flow already commits, per
// variable, exactly what the script's commits will write — a bare commit
// counting as WITH events.
//
// The note tells the author that a bare `commit $X;` now writes the opposite
// of what an older mxcli wrote. For a flow the project already holds that way
// — typically because this very script wrote it on an earlier run — that is
// not true of this run: re-running the script changes nothing about events,
// and exec reports the flow unchanged. Printing the note anyway, on every run
// of an idempotent script, buried the warnings that do apply.
//
// The note stays whenever the run may change what is stored, or cannot be
// told not to: no project (stored == nil), a flow the project does not have
// yet, a plain `create` (its commits are not recorded in prog.FlowCommits —
// it is new code), a stored commit without events that a bare one would flip,
// or a different number of commits of the variable. The matching is the
// same per-variable multiset fmt --upgrade -p uses (mdl/upgrade
// pinCommitEvents), held to equality: anything the upgrade would pin or
// report is left noted here.
func DropSettledCommitNotes(violations []linter.Violation, prog *ast.Program, stored commitEventSource) []linter.Violation {
	if stored == nil || prog == nil {
		return violations
	}
	settled := map[string]bool{}
	isSettled := func(flow string) bool {
		if s, done := settled[flow]; done {
			return s
		}
		s := commitsMatchStored(prog.FlowCommits, flow, stored)
		settled[flow] = s
		return s
	}
	out := violations[:0:0]
	for _, v := range violations {
		if v.RuleID == bareCommitNoteRule && v.Location.DocumentType == "microflow" &&
			isSettled(v.Location.DocumentName) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// commitsMatchStored reports whether every variable the script commits bare
// in flow is committed by the stored flow exactly as the script will write
// it: the same number of commits with events and without.
func commitsMatchStored(commits []ast.FlowCommit, flow string, stored commitEventSource) bool {
	type tally struct{ with, without int }
	script := map[string]*tally{}
	bare := map[string]bool{}
	for _, c := range commits {
		if c.Nanoflow || c.Flow.String() != flow {
			continue
		}
		t := script[c.Variable]
		if t == nil {
			t = &tally{}
			script[c.Variable] = t
		}
		if c.WithoutEvents {
			t.without++
		} else {
			t.with++ // bare or `with events`: both write WithEvents=true
		}
		if c.Bare {
			bare[c.Variable] = true
		}
	}
	if len(bare) == 0 {
		return false // nothing recorded for this flow: a plain create
	}
	events, found := stored.CommitEvents(false, flow)
	if !found {
		return false
	}
	for v := range bare {
		var have tally
		for _, e := range events[v] {
			if e {
				have.with++
			} else {
				have.without++
			}
		}
		if have != *script[v] {
			return false
		}
	}
	return true
}
