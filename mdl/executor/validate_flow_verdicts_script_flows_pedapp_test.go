// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A `create or modify` whose new body calls a flow an earlier statement of the
// same script creates did not build against the stored project, so check -p
// left it unpredicted and said "no flow change exec would refuse" — and exec's
// preflight said nothing either, so exec wrote the earlier statements and then
// refused the flow, leaving the script half-applied. The verdict build now
// resolves calls to flows the script itself declares.

// verdictActStored is the flow as stored: a retrieve whose output a later
// change reads. Moving both into a sub-flow and calling it replaces the
// retrieve while the change still reads $Reporter, which the splice refuses.
const verdictActStored = `mdl 1;
create microflow MyFirstModule.VerdictAct ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
  log info node 'X' 'done';
end;`

const verdictSubCreate = `create microflow MyFirstModule.%s ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
end;`

func verdictActCallingSub(sub string) string {
	return `create or modify microflow MyFirstModule.VerdictAct ($F: FeedbackModule.Feedback)
begin
  call microflow MyFirstModule.` + sub + ` (F = $F);
  log info node 'X' 'done';
end;`
}

func verdictReport(t *testing.T, exec *Executor, src string) FlowVerdictReport {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	return exec.CheckFlowVerdictsReport(prog)
}

func verdictRefusals(vs []linter.Violation) string {
	var says []string
	for _, v := range vs {
		if v.RuleID == FlowRebuildRule && v.Severity == linter.SeverityError {
			says = append(says, v.Message)
		}
	}
	return strings.Join(says, "\n")
}

const stillUsed = "is still used by"

func TestCheckFlowVerdicts_CallToFlowTheScriptCreates(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, verdictActStored); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := agreeExec(t, exec, "mdl 1;\n"+strings.Replace(verdictSubCreate, "%s", "VerdictStoredSub", 1)); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	// Control: the callee is already stored, so the body builds and the
	// refusal is predicted — this already worked.
	t.Run("control: callee stored", func(t *testing.T) {
		r := verdictReport(t, exec, "mdl 1;\n"+verdictActCallingSub("VerdictStoredSub"))
		if got := verdictRefusals(r.Violations); !strings.Contains(got, stillUsed) {
			t.Fatalf("check must predict the refusal %q, reported: %v", stillUsed, r.Violations)
		}
		if len(r.Unpredicted) != 0 {
			t.Errorf("nothing is unpredicted here, reported: %+v", r.Unpredicted)
		}
	})

	// The reported case: the callee is created by the statement before.
	t.Run("callee created earlier in the script", func(t *testing.T) {
		src := "mdl 1;\n" + strings.Replace(verdictSubCreate, "%s", "VerdictNewSub", 1) + "\n" + verdictActCallingSub("VerdictNewSub")
		r := verdictReport(t, exec, src)
		if got := verdictRefusals(r.Violations); !strings.Contains(got, stillUsed) {
			t.Fatalf("check must predict exec's refusal %q, reported violations %v, unpredicted %+v",
				stillUsed, r.Violations, r.Unpredicted)
		}
		if len(r.Unpredicted) != 0 {
			t.Errorf("the change is predicted, yet reported unpredicted: %+v", r.Unpredicted)
		}
		// exec agrees: it writes the sub-flow and then refuses VerdictAct.
		err := agreeExec(t, exec, src)
		if err == nil || !strings.Contains(err.Error(), stillUsed) {
			t.Fatalf("exec must refuse with %q, got: %v", stillUsed, err)
		}
	})

	// A body that depends on something else an earlier statement makes —
	// here a flow a rename brings into being, which has no declaration to
	// resolve to — is still not predicted, and is now reported as such
	// instead of passing silently.
	t.Run("unpredictable change is reported", func(t *testing.T) {
		src := "mdl 1;\nrename microflow MyFirstModule.VerdictStoredSub to VerdictRenamedSub;\n" +
			verdictActCallingSub("VerdictRenamedSub")
		r := verdictReport(t, exec, src)
		if len(r.Unpredicted) != 1 || r.Unpredicted[0].Name != "MyFirstModule.VerdictAct" {
			t.Fatalf("want VerdictAct reported unpredicted, got %+v (violations %v)", r.Unpredicted, r.Violations)
		}
		if !strings.Contains(r.Unpredicted[0].Reason, "VerdictRenamedSub") {
			t.Errorf("the reason must name what the body depends on, got %q", r.Unpredicted[0].Reason)
		}
	})
}
