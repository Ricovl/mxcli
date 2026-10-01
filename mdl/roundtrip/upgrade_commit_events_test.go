// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// Rehearsal M1 (ako/mxcli#873): a flow stored by an mxcli older than #895,
// when a bare `commit` meant without events. The same script now means WITH
// events, so re-running it changes the stored flow — and inside a loop the
// mdl 1 splice refuses it on every run. `fmt --upgrade -p` reads the stored
// flag and writes it into the script; the upgraded script then re-runs as
// Unchanged, twice, under both language versions.
//
// The repro is the rehearsal's commit-default-in-loop.setup.mdl + .mdl.
func TestUpgradePinsStoredCommitEvents_ReRunIsUnchanged(t *testing.T) {
	const setup = `create or modify persistent entity MyFirstModule.Tx (
  Amount: Decimal
);
create or modify microflow MyFirstModule.Repro_CommitInLoop ($Items: List of MyFirstModule.Tx)
begin
  loop $T in $Items
  begin
    change $T (Amount = 0);
    commit $T without events;
  end loop;
end;
`
	const script = `create or modify microflow MyFirstModule.Repro_CommitInLoop ($Items: List of MyFirstModule.Tx)
begin
  loop $T in $Items
  begin
    change $T (Amount = 0);
    commit $T;
  end loop;
end;
`
	h := newHarness(t)
	defer h.close()

	for _, header := range []string{"", "mdl 1;\n"} {
		name := map[string]string{"": "mdl 0", "mdl 1;\n": "mdl 1"}[header]
		t.Run(name, func(t *testing.T) {
			h.restore()
			if err := h.exec(setup); err != nil {
				t.Fatalf("setup: %v\n%s", err, h.out.String())
			}
			stored := h.snapshot()

			// Control: the script as written does not re-run as Unchanged
			// against this flow — it is refused (mdl 1) or rewrites the flow
			// with events on (mdl 0). Without this the checks below could pass
			// against a build where the bare commit still meant without events.
			err := h.exec(header + script)
			if err == nil && len(stored.diff(h.snapshot())) == 0 {
				t.Fatalf("control: the unpinned script re-ran as Unchanged:\n%s", h.out.String())
			}
			t.Logf("control (%s): err=%v", name, err)
			h.restore()
			if err := h.exec(setup); err != nil {
				t.Fatalf("setup: %v", err)
			}
			stored = h.snapshot()

			res, err := upgrade.Upgrade(header+script, upgrade.Options{
				Commits: executor.NewStoredCommitEvents(h.exe.Backend()),
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.CommitsPinned != 1 || !strings.Contains(res.Source, "    commit $T without events;\n") {
				t.Fatalf("the upgrade did not pin the stored flag (%d pinned):\n%s", res.CommitsPinned, res.Source)
			}
			for run := 1; run <= 2; run++ {
				if err := h.exec(res.Source); err != nil {
					t.Fatalf("run %d of the upgraded script: %v\n%s", run, err, h.out.String())
				}
				if !strings.Contains(h.out.String(), "Unchanged ") {
					t.Errorf("run %d did not report Unchanged:\n%s", run, h.out.String())
				}
				if changed := stored.diff(h.snapshot()); len(changed) != 0 {
					t.Errorf("run %d wrote %d unit(s):\n  %s", run, len(changed), strings.Join(changed, "\n  "))
				}
			}
		})
	}
}
