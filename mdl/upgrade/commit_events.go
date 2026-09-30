// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// StoredCommits is the project a script runs against, as far as the commit
// pin needs it (ako/mxcli#873): the events flag of each Commit activity a
// stored flow holds. `fmt --upgrade -p app.mpr` passes one backed by the
// project; tests pass a map.
type StoredCommits interface {
	// CommitEvents returns, per committed variable (without the `$`), the
	// WithEvents flag of each Commit activity in the stored microflow (the
	// nanoflow when nanoflow is set) named qualifiedName, loop bodies included.
	// found is false when the project has no such flow.
	CommitEvents(nanoflow bool, qualifiedName string) (events map[string][]bool, found bool)
}

// Note is something the upgrade did not change and the author should know:
// printed by fmt, never an error.
type Note struct {
	Line    int
	Code    string // the rule the note is about, e.g. MDL067
	Message string
}

// commitNoteCode is the check that reports the #895 default change.
const commitNoteCode = "MDL067"

// pinCommitEvents states the stored events flag on the bare commits of every
// `create or modify` flow whose stored flow commits the variable without
// events (ako/mxcli#873).
//
// A bare `commit $X;` means WITH events since #895, Studio Pro's default; an
// older mxcli stored the same statement WITHOUT. Upgrading a script must not
// change what it builds, and re-running it against its own project must not
// either, so where the project says the stored commit has no events, the
// statement says so: `commit $X without events;`. A new flow, a new commit, or
// a stored commit with events is left as written: the current default already
// describes it.
//
// The script's commits are matched to the stored Commit activities by
// variable. A commit whose flag is written matches a stored activity with the
// same flag; the bare ones are pinned only when every remaining stored commit
// of the variable is without events and there are no more bare ones than
// stored ones. Anything else — the stored flow commits the variable both ways,
// or the script adds a commit — cannot be matched by variable alone, so it is
// left and reported. Without a project, every flow with a bare commit is
// reported.
func pinCommitEvents(prog *ast.Program, stored StoredCommits) (edits []Edit, pinned int, notes []Note) {
	type flowKey struct {
		nanoflow bool
		name     string
	}
	var order []flowKey
	byFlow := map[flowKey][]ast.FlowCommit{}
	for _, c := range prog.FlowCommits {
		k := flowKey{c.Nanoflow, c.Flow.String()}
		if _, seen := byFlow[k]; !seen {
			order = append(order, k)
		}
		byFlow[k] = append(byFlow[k], c)
	}

	for _, k := range order {
		commits := byFlow[k]
		var bare []ast.FlowCommit
		for _, c := range commits {
			if c.Bare {
				bare = append(bare, c)
			}
		}
		if len(bare) == 0 {
			continue
		}
		if stored == nil {
			notes = append(notes, Note{Line: bare[0].Line, Code: commitNoteCode, Message: fmt.Sprintf(
				"%s: %s mean%s WITH events since #895 (%s); if an older mxcli stored this flow, it holds them "+
					"without events — pass -p app.mpr to state the stored flag, or write `with events` / "+
					"`without events` by hand",
				k.name, countOf(len(bare), "bare `commit` statement"), plural(len(bare), "s", ""), commitNoteCode)})
			continue
		}
		events, found := stored.CommitEvents(k.nanoflow, k.name)
		if !found {
			continue // a new flow: the default describes it
		}
		for _, v := range variablesOf(bare) {
			remaining := append([]bool(nil), events[v]...)
			var mine []ast.FlowCommit
			total, unmatched := 0, 0
			for _, c := range commits {
				if c.Variable != v {
					continue
				}
				total++
				if c.Bare {
					mine = append(mine, c)
					continue
				}
				var ok bool
				if remaining, ok = removeOne(remaining, !c.WithoutEvents); !ok {
					unmatched++ // a commit the stored flow does not have with this flag
				}
			}
			withEvents, without := 0, 0
			for _, e := range remaining {
				if e {
					withEvents++
				} else {
					without++
				}
			}
			switch {
			case without == 0:
				// Stored with events, or not stored: the default describes it.
			case withEvents == 0 && len(mine)+unmatched <= without:
				for _, c := range mine {
					edits = append(edits, c.PinWithoutEvents.Edits...)
					pinned++
				}
			default:
				notes = append(notes, Note{Line: mine[0].Line, Code: commitNoteCode, Message: fmt.Sprintf(
					"%s: the stored flow commits $%s %s, and the script commits it %s, %d bare; which bare "+
						"commit is which stored one cannot be told, and since #895 (%s) a bare commit means WITH "+
						"events — write `with events` or `without events` on each by hand",
					k.name, v, storedSummary(len(events[v])-countTrue(events[v]), countTrue(events[v])),
					times(total), len(mine), commitNoteCode)})
			}
		}
	}
	return edits, pinned, notes
}

// variablesOf lists the committed variables in first-use order.
func variablesOf(cs []ast.FlowCommit) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range cs {
		if !seen[c.Variable] {
			seen[c.Variable] = true
			out = append(out, c.Variable)
		}
	}
	return out
}

// removeOne removes one occurrence of v from s, and reports whether there
// was one.
func removeOne(s []bool, v bool) ([]bool, bool) {
	for i, e := range s {
		if e == v {
			return append(s[:i], s[i+1:]...), true
		}
	}
	return s, false
}

func countTrue(s []bool) int {
	n := 0
	for _, e := range s {
		if e {
			n++
		}
	}
	return n
}

func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return fmt.Sprintf("%d times", n)
}

// storedSummary says how often a variable is committed without and with
// events.
func storedSummary(without, withEvents int) string {
	var parts []string
	if without > 0 {
		parts = append(parts, times(without)+" without events")
	}
	if withEvents > 0 {
		parts = append(parts, times(withEvents)+" with events")
	}
	return strings.Join(parts, " and ")
}

func countOf(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
