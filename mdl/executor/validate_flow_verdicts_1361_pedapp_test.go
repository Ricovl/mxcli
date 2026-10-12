// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1361: "check --references passes a create or modify
// microflow that exec then refuses with 'Nothing was written'" when the new
// body calls a microflow the same script creates. The reporter hit it with two
// refusals —
//
//	replace $Equal: the fragment declares $Different, which the microflow already has
//	the Loop at (620, 200) changes inside its body
//
// — both predicted when the body calls nothing new (variant A), both silently
// passed when it calls a flow an earlier statement creates (variant B), since
// that body did not build against the stored project. The build now resolves
// calls to the script's own flows (TestCheckFlowVerdicts_CallToFlowTheScriptCreates
// covers the "still used by" refusal); these pin the two shapes reported.

const issue1361NewSub = "create microflow MyFirstModule.%s () begin log info node 'X' 'sub'; end;\n"

// issue1361Variants returns the A (no call) and B (calls a flow the script
// creates first) form of a `create or modify` whose body has %s where the
// call goes.
func issue1361Variants(modify, sub string) []struct{ name, src string } {
	return []struct{ name, src string }{
		{"A: calls nothing new", "mdl 1;\n" + strings.Replace(modify, "%s", "", 1)},
		{"B: calls a flow the script creates", "mdl 1;\n" + strings.Replace(issue1361NewSub, "%s", sub, 1) +
			strings.Replace(modify, "%s", "call microflow MyFirstModule."+sub+" ();", 1)},
	}
}

func issue1361Expect(t *testing.T, exec *Executor, modify, sub, refusal string) {
	t.Helper()
	for _, tc := range issue1361Variants(modify, sub) {
		t.Run(tc.name, func(t *testing.T) {
			r := verdictReport(t, exec, tc.src)
			if got := verdictRefusals(r.Violations); !strings.Contains(got, refusal) {
				t.Fatalf("check must predict exec's refusal %q, reported violations %v, unpredicted %+v",
					refusal, r.Violations, r.Unpredicted)
			}
			if len(r.Unpredicted) != 0 {
				t.Errorf("the change is predicted, yet reported unpredicted: %+v", r.Unpredicted)
			}
		})
	}
}

const issue1361VarStored = `mdl 1;
create microflow MyFirstModule.Issue1361Var ()
begin
  declare $Equal boolean = true;
  declare $Different boolean = false;
  log info node 'X' 'done';
end;`

const issue1361VarModify = `create or modify microflow MyFirstModule.Issue1361Var ()
begin
  %s
  declare $Different boolean = true;
  log info node 'X' 'done';
end;`

func TestCheckFlowVerdicts_Issue1361_VariableClash(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, issue1361VarStored); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	issue1361Expect(t, exec, issue1361VarModify, "Issue1361VarSub",
		"replace $Equal: the fragment declares $Different, which the microflow already has")
}

const issue1361LoopStored = `mdl 1;
create microflow MyFirstModule.Issue1361Loop ($Items: List of FeedbackModule.Feedback)
begin
  declare $S string = '';
  loop $It in $Items
  begin
    set $S = $S + 'a';
    commit $It;
  end loop;
  log info node 'X' $S;
end;`

const issue1361LoopModify = `create or modify microflow MyFirstModule.Issue1361Loop ($Items: List of FeedbackModule.Feedback)
begin
  declare $S string = '';
  loop $It in $Items
  begin
    set $S = $S + 'a';
    log info node 'X' 'added in loop';
    commit $It;
  end loop;
  %s
  log info node 'X' $S;
end;`

func TestCheckFlowVerdicts_Issue1361_LoopBody(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, issue1361LoopStored); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	issue1361Expect(t, exec, issue1361LoopModify, "Issue1361LoopSub", "changes inside its body")
}
