// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Item 4 of saingge's comment on mendixlabs/mxcli#1517 (ako/mxcli#1399):
// `check` reported "Check passed!" with a "not predicted" note for a statement
// on a flow an earlier statement of the script had already changed, and exec
// then refused that statement and rolled the whole script back ("Nothing was
// written: statement 1 was rolled back").
//
// check predicted each flow statement against the flow as stored, so after an
// earlier change to the same flow it gave up. It now applies each change it
// predicts as spliced to a simulated copy — held in memory, never written —
// so the next statement on that flow is predicted against what exec would see.

const chainedBase = `mdl 1;
create or modify microflow MyFirstModule.Chained ($Items: List of FeedbackModule.Feedback)
begin
  log info node 'X' 'start';
  loop $It in $Items
  begin
    log info node 'X' 'in loop';
  end loop;
  log info node 'X' 'done';
end;`

// chainedStep1 changes the flow beside the loop: spliced.
var chainedStep1 = strings.Replace(chainedBase, "'start'", "'begin'", 1)

func TestCheckFlowVerdicts_ChainedOnTheSameFlow(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, chainedBase); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	report := func(t *testing.T, src string) FlowVerdictReport {
		t.Helper()
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		before := projectFiles(t, dir)
		r := exec.CheckFlowVerdictsReport(prog)
		after := projectFiles(t, dir)
		for f, h := range before {
			if after[f] != h {
				t.Errorf("check wrote %s", f)
			}
		}
		if len(after) != len(before) {
			t.Errorf("check changed the project's files: %d -> %d", len(before), len(after))
		}
		return r
	}

	t.Run("a refusal after an earlier change is predicted", func(t *testing.T) {
		// Step 2 changes the loop body's shape: refused, whatever step 1 did.
		step2 := strings.Replace(chainedStep1, "'in loop';", "'in loop';\n    log info node 'X' 'again';", 1)
		r := report(t, chainedStep1+"\n"+strings.TrimPrefix(step2, "mdl 1;\n"))
		if len(r.Violations) != 1 || !strings.Contains(r.Violations[0].Message, "changes inside its body") {
			t.Errorf("want statement 2's refusal predicted, got violations %v, unpredicted %+v", r.Violations, r.Unpredicted)
		}
		if len(r.Unpredicted) != 0 {
			t.Errorf("reported unpredicted: %+v", r.Unpredicted)
		}
	})
	t.Run("an alter after an earlier change is predicted", func(t *testing.T) {
		// The alter addresses the log step 1 rewrote: against the stored flow
		// it would not resolve, against the simulated one it does.
		alter := "alter microflow MyFirstModule.Chained {\n  replace log node 'X' 'begin' with begin log info node 'X' 'go'; end;\n};"
		r := report(t, chainedStep1+"\n"+alter)
		if len(r.Violations) != 0 || len(r.Unpredicted) != 0 {
			t.Errorf("want the alter predicted to apply, got violations %v, unpredicted %+v", r.Violations, r.Unpredicted)
		}
	})
	t.Run("control: a refusal of the first statement is still reported", func(t *testing.T) {
		step := strings.Replace(chainedBase, "'in loop';", "'in loop';\n    log info node 'X' 'again';", 1)
		r := report(t, step)
		if len(r.Violations) != 1 {
			t.Errorf("want the refusal, got %v", r.Violations)
		}
	})
	// The simulation is discarded: a later check sees the flow as stored.
	t.Run("control: the simulated change does not outlive the check", func(t *testing.T) {
		if desc := describeFlow(t, exec, "Chained"); !strings.Contains(desc, "'start'") {
			t.Errorf("the stored flow reads as changed after check:\n%s", desc)
		}
	})
	// exec agrees with the first prediction: it refuses statement 2. (Run
	// last: ExecuteProgram writes statement 1; the rollback is cmd exec's.)
	t.Run("exec refuses what check predicted", func(t *testing.T) {
		step2 := strings.Replace(chainedStep1, "'in loop';", "'in loop';\n    log info node 'X' 'again';", 1)
		err := agreeExec(t, exec, chainedStep1+"\n"+strings.TrimPrefix(step2, "mdl 1;\n"))
		if err == nil || !strings.Contains(err.Error(), "changes inside its body") {
			t.Errorf("want exec to refuse statement 2, got %v", err)
		}
	})
}
