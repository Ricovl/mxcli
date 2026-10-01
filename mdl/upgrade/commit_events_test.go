// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"strings"
	"testing"
)

// storedFlows is an upgrade.StoredCommits with fixed answers: flow name ->
// variable -> the WithEvents flag of each stored Commit activity.
type storedFlows map[string]map[string][]bool

func (s storedFlows) CommitEvents(_ bool, qn string) (map[string][]bool, bool) {
	e, ok := s[qn]
	return e, ok
}

// The rehearsal's M1 repro (ako/mxcli#873): the stored flow — built by an
// older mxcli, when a bare commit meant without events — commits $T without
// events inside a loop; the script, unchanged, now means WITH events. With the
// project, the upgrade states the stored flag, so the script builds what is
// stored and the re-run is Unchanged instead of a refused loop-body change.
const commitInLoop = `create or modify microflow MyFirstModule.Repro_CommitInLoop ($Items: List of MyFirstModule.Tx)
begin
  loop $T in $Items
  begin
    change $T (Amount = 0);
    commit $T;
  end loop;
end;
`

func TestUpgrade_PinsTheStoredCommitWithoutEvents(t *testing.T) {
	stored := storedFlows{"MyFirstModule.Repro_CommitInLoop": {"T": {false}}}
	res := mustUpgrade(t, commitInLoop, Options{Commits: stored})
	want := strings.Replace(commitInLoop, "commit $T;", "commit $T without events;", 1)
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.CommitsPinned != 1 || !res.Changed() {
		t.Errorf("CommitsPinned = %d, Changed = %v; want 1, true", res.CommitsPinned, res.Changed())
	}
	if len(res.Notes) != 0 {
		t.Errorf("Notes = %+v, want none", res.Notes)
	}
	// Idempotent: the pinned statement is no longer bare.
	if again := mustUpgrade(t, res.Source, Options{Commits: stored}); again.Changed() || len(again.Notes) != 0 {
		t.Errorf("second upgrade: changed=%v notes=%+v", again.Changed(), again.Notes)
	}
	// With the header too: the pin is not gated.
	hdr := mustUpgrade(t, commitInLoop, Options{AddHeader: true, Commits: stored})
	if !strings.Contains(hdr.Source, "commit $T without events;") || !hdr.HeaderAdded {
		t.Errorf("with the header:\n%s", hdr.Source)
	}
}

// The controls: the pin follows the stored flag, never the script alone.
func TestUpgrade_CommitPinFollowsTheStoredFlow(t *testing.T) {
	two := `create or modify microflow M.F ($A: M.E, $B: M.E)
begin
  commit $A;
  COMMIT $B REFRESH;
  commit $A with events;
end;
create or modify nanoflow M.N ($A: M.E) begin commit $A; end;
create microflow M.New ($A: M.E) begin commit $A; end;
alter microflow M.F {
  insert after $A begin commit $B; end;
};
`
	for _, c := range []struct {
		name   string
		stored storedFlows
		want   []string // substrings of the output
		pinned int
		notes  []string
	}{
		{"stored with events: left as written",
			storedFlows{"M.F": {"A": {true, true}, "B": {true}}, "M.N": {"A": {true}}},
			[]string{"  commit $A;\n", "  COMMIT $B REFRESH;\n"}, 0, nil},
		{"the flow is not in the project: left as written",
			storedFlows{}, []string{"  commit $A;\n"}, 0, nil},
		{"the variable is not committed in the stored flow: left as written",
			storedFlows{"M.F": {"X": {false}}}, []string{"  commit $A;\n"}, 0, nil},
		{"stored without events: pinned, in the keyword's case, before refresh",
			storedFlows{"M.F": {"A": {false, true}, "B": {false}}, "M.N": {"A": {false}}},
			[]string{"  commit $A without events;\n", "  COMMIT $B WITHOUT EVENTS REFRESH;\n",
				"  commit $A with events;\n", "begin commit $A without events; end;\ncreate microflow M.New ($A: M.E) begin commit $A; end;",
				"begin commit $B; end;"}, 3, nil},
		{"stored both ways for the bare ones: reported",
			storedFlows{"M.F": {"A": {false, true, true}, "B": {true}}},
			[]string{"  commit $A;\n"}, 0, []string{"M.F: the stored flow commits $A once without events and twice with events, and the script commits it twice, 1 bare"}},
		{"the script commits more often than the stored flow: reported",
			storedFlows{"M.F": {"A": {false}, "B": {false}}},
			[]string{"  commit $A;\n", "  COMMIT $B WITHOUT EVENTS REFRESH;\n"}, 1,
			[]string{"M.F: the stored flow commits $A once without events, and the script commits it twice, 1 bare"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := mustUpgrade(t, two, Options{Commits: c.stored})
			for _, w := range c.want {
				if !strings.Contains(res.Source, w) {
					t.Errorf("want %q in:\n%s", w, res.Source)
				}
			}
			if res.CommitsPinned != c.pinned {
				t.Errorf("CommitsPinned = %d, want %d", res.CommitsPinned, c.pinned)
			}
			var got []string
			for _, n := range res.Notes {
				got = append(got, n.Message)
				if n.Code != "MDL067" {
					t.Errorf("note code %q, want MDL067", n.Code)
				}
			}
			if len(got) != len(c.notes) {
				t.Fatalf("notes %q, want %d", got, len(c.notes))
			}
			for i, w := range c.notes {
				if !strings.Contains(got[i], w) {
					t.Errorf("note %q does not contain %q", got[i], w)
				}
			}
		})
	}
}

// Without a project the script is left as written, and each flow with a bare
// commit is reported once, naming MDL067 and -p.
func TestUpgrade_BareCommitWithoutProjectIsReported(t *testing.T) {
	res := mustUpgrade(t, commitInLoop+"create or modify microflow M.G ($A: M.E) begin commit $A without events; end;\n", Options{})
	if res.Changed() || res.CommitsPinned != 0 {
		t.Errorf("changed without a project:\n%s", res.Source)
	}
	if len(res.Notes) != 1 {
		t.Fatalf("notes = %+v, want one", res.Notes)
	}
	n := res.Notes[0]
	for _, w := range []string{"MyFirstModule.Repro_CommitInLoop", "1 bare `commit` statement means WITH events", "MDL067", "-p"} {
		if !strings.Contains(n.Message, w) {
			t.Errorf("note %q does not mention %q", n.Message, w)
		}
	}
	if n.Line != 6 {
		t.Errorf("note line %d, want 6", n.Line)
	}
}
