// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#942: `commit $L with events` and a bare `commit $L` are one stored
// activity (events on), and describe prints the bare form. The statement diff
// compared the AST flag that records the redundant `with events` was written,
// so the declared commit never matched its own stored activity:
//
//   - every re-exec of an unchanged script reported "Unchanged microflow …
//     (spliced: 1 replaced)" — and replaced the commit;
//   - a change inside a loop body next to such a commit made the diff run the
//     loop and the commit together, which bypassed the "changes inside its
//     body" refusal: under mdl 1 the loop was silently rebuilt with new $IDs.
//
// The bare-commit spelling is the control for both: it always behaved.
func commitEventsFlow(name, commit, value string) string {
	return fmt.Sprintf(`create or modify microflow MyFirstModule.%s ($L: List of System.User)
begin
  log info node 'N' 'start';
  loop $U in $L
  begin
    change $U (Name = '%s');
  end loop;
  %s;
end;
`, name, value, commit)
}

func TestSpliceRerun_CommitWithEventsIsTheStoredCommit(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	for _, c := range []struct{ name, flow, commit string }{
		{"with events", "Repro_CommitWithEvents", "commit $L with events"},
		{"control: bare commit", "Repro_CommitBare", "commit $L"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h.restore()
			script := "mdl 1;\n" + commitEventsFlow(c.flow, c.commit, "a")
			if err := h.exec(script); err != nil {
				t.Fatalf("create: %v\n%s", err, h.out.String())
			}
			// A real change outside the loop is spliced: one statement.
			edited := strings.Replace(script, "'start'", "'begin'", 1)
			before := h.snapshot()
			if err := h.exec(edited); err != nil {
				t.Fatalf("change: %v\n%s", err, h.out.String())
			}
			if !strings.Contains(h.out.String(), "(spliced: 1 replaced)") {
				t.Errorf("change: want one statement replaced:\n%s", h.out.String())
			}
			if len(before.diff(h.snapshot())) == 0 {
				t.Fatal("change: the real change wrote nothing")
			}
			// Re-executing it writes nothing and splices nothing.
			assertUnchangedRerun(t, h, edited)
			// The same where the flow is drawn other than the builder would
			// draw it — as Studio Pro, or a splice, leaves it — so that it is
			// the statement diff that decides, not the built comparison: the
			// commit moved, which a script without @position does not state.
			drawn := strings.Replace(edited, "  "+c.commit+";", "  @position(900, 320)\n  "+c.commit+";", 1)
			if err := h.exec(drawn); err != nil {
				t.Fatalf("move the commit: %v\n%s", err, h.out.String())
			}
			if !strings.Contains(h.describeUnder("mdl 1;", "microflow MyFirstModule."+c.flow), "@position(900, 320)") {
				t.Fatalf("the commit was not moved:\n%s", h.out.String())
			}
			assertUnchangedRerun(t, h, edited)

			// A change inside the loop body is refused under mdl 1, nothing
			// written — the commit next to the loop must not turn it into a
			// replace of the loop.
			stored := drawnObjects(t, h.flowUnit(t, c.flow))
			before = h.snapshot()
			err := h.exec("mdl 1;\n" + strings.Replace(commitEventsFlow(c.flow, c.commit, "z"), "'start'", "'begin'", 1))
			if err == nil || !strings.Contains(err.Error(), "changes inside its body") {
				t.Errorf("a loop-body change: want the refusal naming the loop, got %v\n%s", err, h.out.String())
			}
			if changed := before.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("the refused loop-body change wrote: %v\n%s", changed, h.out.String())
			}
			after := drawnObjects(t, h.flowUnit(t, c.flow))
			for id := range stored {
				if _, ok := after[id]; !ok {
					t.Errorf("stored object %s was renumbered away", id)
				}
			}
		})
	}
}

// assertUnchangedRerun runs script twice: neither run may write or splice.
func assertUnchangedRerun(t *testing.T, h *harness, script string) {
	t.Helper()
	for run := 1; run <= 2; run++ {
		before := h.snapshot()
		if err := h.exec(script); err != nil {
			t.Fatalf("re-run %d: %v\n%s", run, err, h.out.String())
		}
		if changed := before.diff(h.snapshot()); len(changed) != 0 {
			t.Errorf("re-run %d of the unchanged script wrote: %v\n%s", run, changed, h.out.String())
		}
		if out := h.out.String(); strings.Contains(out, "spliced") || !strings.Contains(out, "Unchanged microflow") {
			t.Errorf("re-run %d must be Unchanged with nothing spliced:\n%s", run, out)
		}
	}
}

