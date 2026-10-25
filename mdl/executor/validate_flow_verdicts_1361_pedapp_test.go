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

// The first shape is no longer a refusal (ako/mxcli#1399): the patch replaces
// $Equal with the new $Different and drops the stored one, and the clash was
// judged against the stored $Different that the same patch drops. A body that
// builds declares each variable once, so `create or modify` cannot clash with
// a stored activity it keeps. Both variants are now predicted as spliced, and
// exec writes the declared flow; the start event, the log and the end event
// keep their $IDs.
func TestCheckFlowVerdicts_Issue1361_VariableClash(t *testing.T) {
	for _, tc := range issue1361Variants(issue1361VarModify, "Issue1361VarSub") {
		t.Run(tc.name, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			if err := agreeExec(t, exec, issue1361VarStored); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			before := flowObjectIDs(loopEditFlowOf(t, exec, "Issue1361Var").ObjectCollection)
			r := verdictReport(t, exec, tc.src)
			if got := verdictRefusals(r.Violations); got != "" || len(r.Unpredicted) != 0 {
				t.Fatalf("check must predict the splice, reported refusals %q, unpredicted %+v", got, r.Unpredicted)
			}
			if err := agreeExec(t, exec, tc.src); err != nil {
				t.Fatalf("exec must splice the change: %v", err)
			}
			got := describeFlow(t, exec, "Issue1361Var")
			if strings.Contains(got, "$Equal") || strings.Count(got, "declare $Different") != 1 ||
				!strings.Contains(got, "declare $Different Boolean = true;") {
				t.Errorf("the declared flow was not written:\n%s", got)
			}
			kept := 0
			for id := range flowObjectIDs(loopEditFlowOf(t, exec, "Issue1361Var").ObjectCollection) {
				if _, ok := before[id]; ok {
					kept++
				}
			}
			if kept < 3 {
				t.Errorf("want the start event, the log and the end event to keep their $IDs, %d did", kept)
			}
		})
	}
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
