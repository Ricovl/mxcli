// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1517: an alter block is one change, so its scope checks
// have to judge the flow the whole block writes — not each operation against
// the flow as stored, nor what the formatter happens to print of a fragment.

const scopeHelperFlow = `mdl 1;
create microflow MyFirstModule.ScopeHelper () returns String
begin
  return 'x';
end;`

const scopeRenameFlow = `mdl 1;
create microflow MyFirstModule.ScopeRename () returns String
begin
  $Code = call microflow MyFirstModule.ScopeHelper();
  log info node 'T' $Code;
  return $Code;
end;`

const scopeLoopFlow = `mdl 1;
create microflow MyFirstModule.ScopeLoop ($Items: List of FeedbackModule.Feedback)
begin
  log info node 'T' 'start';
  loop $It in $Items
  begin
    commit $It;
  end loop;
  log info node 'T' 'done';
end;`

func openScopeFixture(t *testing.T) *Executor {
	t.Helper()
	exec, out, _ := openPedAppCopy(t)
	for _, src := range []string{scopeHelperFlow, scopeRenameFlow, scopeLoopFlow} {
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
	}
	return exec
}

// scopeAgree runs check's flow verdict, then exec, and asserts both refuse
// (want != "", found in both) or both accept.
func scopeAgree(t *testing.T, exec *Executor, src, want string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var says []string
	for _, v := range exec.CheckFlowVerdicts(prog) {
		says = append(says, v.Message)
	}
	checkSays := strings.Join(says, "\n")
	execErr := exec.ExecuteProgram(prog)
	if want == "" {
		if checkSays != "" || execErr != nil {
			t.Errorf("check and exec must both accept\ncheck: %s\nexec: %v", checkSays, execErr)
		}
		return
	}
	if !strings.Contains(checkSays, want) {
		t.Errorf("check must predict the refusal %q, reported: %s", want, checkSays)
	}
	if execErr == nil || !strings.Contains(execErr.Error(), want) {
		t.Errorf("exec must refuse with %q, got: %v", want, execErr)
	}
}

// A2: a consistent rename of a variable — its producer, every reader and the
// return, in one block — was refused "$Code is still used by: log …; return
// $Code", because each operation was judged against readers that later
// operations of the same block replace.
func TestAlterMicroflow_ScopeRenameAcrossTheBlock(t *testing.T) {
	const rename = `mdl 1;
alter microflow MyFirstModule.ScopeRename {
  replace $Code = call microflow MyFirstModule.ScopeHelper() with begin $Code2 = call microflow MyFirstModule.ScopeHelper(); end;
  replace log node 'T' * with begin log info node 'T' $Code2; end;
  replace return $Code with begin return $Code2; end;
};`
	t.Run("the whole rename is accepted by check and exec", func(t *testing.T) {
		exec := openScopeFixture(t)
		scopeAgree(t, exec, rename, "")
		got := describeFlow(t, exec, "ScopeRename")
		if strings.Contains(got, "$Code;") || strings.Contains(got, "$Code\n") || !strings.Contains(got, "return $Code2;") {
			t.Errorf("the rename was not written:\n%s", got)
		}
	})
	// Controls: a reader that the block leaves in place still blocks the
	// rename, whichever order the operations come in.
	for name, src := range map[string]string{
		"the return still reads $Code": `mdl 1;
alter microflow MyFirstModule.ScopeRename {
  replace $Code = call microflow MyFirstModule.ScopeHelper() with begin $Code2 = call microflow MyFirstModule.ScopeHelper(); end;
  replace log node 'T' * with begin log info node 'T' $Code2; end;
};`,
		"the log still reads $Code, the rename comes last": `mdl 1;
alter microflow MyFirstModule.ScopeRename {
  replace return $Code with begin return 'y'; end;
  replace $Code = call microflow MyFirstModule.ScopeHelper() with begin $Code2 = call microflow MyFirstModule.ScopeHelper(); end;
};`,
		"a fragment of the block reads $Code": `mdl 1;
alter microflow MyFirstModule.ScopeRename {
  insert before log node 'T' * begin log info node 'T' $Code; end;
  replace $Code = call microflow MyFirstModule.ScopeHelper() with begin $Code2 = call microflow MyFirstModule.ScopeHelper(); end;
  replace log node 'T' * with begin log info node 'T' $Code2; end;
  replace return $Code with begin return $Code2; end;
};`,
	} {
		t.Run("control: "+name, func(t *testing.T) {
			exec := openScopeFixture(t)
			scopeAgree(t, exec, src, "$Code")
		})
	}
}

// A2 through `create or modify`, which splices with the same operations: a
// retrieve and its only reader moved into a sub-flow were refused "$Reporter
// is still used by: change $F …", by the reader the same patch drops.
func TestCreateOrModify_ScopeProducerAndReaderMovedOut(t *testing.T) {
	exec := openScopeFixture(t)
	for _, src := range []string{`mdl 1;
create microflow MyFirstModule.ScopeMoved ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
  log info node 'X' 'done';
end;`, `mdl 1;
create microflow MyFirstModule.ScopeMovedSub ($F: FeedbackModule.Feedback)
begin
  retrieve $Reporter from System.User first;
  change $F (Subject = $Reporter/Name);
end;`} {
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	scopeAgree(t, exec, `mdl 1;
create or modify microflow MyFirstModule.ScopeMoved ($F: FeedbackModule.Feedback)
begin
  call microflow MyFirstModule.ScopeMovedSub (F = $F);
  log info node 'X' 'done';
end;`, "")
	got := describeFlow(t, exec, "ScopeMoved")
	if strings.Contains(got, "$Reporter") || !strings.Contains(got, "call microflow MyFirstModule.ScopeMovedSub") {
		t.Errorf("the change was not written:\n%s", got)
	}
}

// B2: a declaration made by an earlier operation of the block is on the path
// of a later one that reads it, when it is upstream of where that one goes.
func TestAlterMicroflow_ScopeDeclaredByAnEarlierOperation(t *testing.T) {
	t.Run("a later fragment reads a variable an earlier insert declares", func(t *testing.T) {
		exec := openScopeFixture(t)
		scopeAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'start' begin declare $Late Integer = 0; end;
  replace log node 'T' 'done' with begin log info node 'T' toString($Late); end;
};`, "")
		if got := describeFlow(t, exec, "ScopeLoop"); !strings.Contains(got, "toString($Late)") {
			t.Errorf("the replacement was not written:\n%s", got)
		}
	})
	t.Run("an insert before the reader's target declares it", func(t *testing.T) {
		exec := openScopeFixture(t)
		scopeAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert before log node 'T' 'done' begin declare $Late Integer = 0; end;
  replace log node 'T' 'done' with begin log info node 'T' toString($Late); end;
};`, "")
		got := describeFlow(t, exec, "ScopeLoop")
		if d, r := strings.Index(got, "declare $Late"), strings.Index(got, "toString($Late)"); d < 0 || r < d {
			t.Errorf("the declaration is not written before its reader:\n%s", got)
		}
	})
	// Controls: a declaration the block makes downstream of the reader is
	// not on its path.
	for name, src := range map[string]string{
		"declared after the reader": `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'done' begin declare $Late Integer = 0; end;
  insert after log node 'T' 'start' begin log info node 'T' toString($Late); end;
};`,
		"two inserts after the same node": `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'start' begin declare $Late Integer = 0; end;
  insert after log node 'T' 'start' begin log info node 'T' toString($Late); end;
};`,
		"declared after the replaced reader": `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'done' begin declare $Late Integer = 0; end;
  replace log node 'T' 'done' with begin log info node 'T' toString($Late); end;
};`,
	} {
		t.Run("control: "+name, func(t *testing.T) {
			exec := openScopeFixture(t)
			scopeAgree(t, exec, src, "not declared on the path")
		})
	}
}

// B3: a replaced loop's body was never scope-checked — the formatter renders
// a loop as its header only — so a body reading an undeclared variable was
// "Altered" and failed mx check with CE0109 "Undefined variable".
func TestAlterMicroflow_ScopeReplaceLoopChecksTheBody(t *testing.T) {
	t.Run("a body reading an undeclared variable is refused", func(t *testing.T) {
		exec := openScopeFixture(t)
		scopeAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  replace loop $It in $Items with begin
    loop $It in $Items
    begin
      log info node 'T' $Nope;
    end loop;
  end;
};`, "$Nope")
	})
	t.Run("control: the iterator and what the body declares are in scope", func(t *testing.T) {
		exec := openScopeFixture(t)
		scopeAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  replace loop $It in $Items with begin
    loop $It in $Items
    begin
      declare $N Integer = 1;
      log info node 'T' toString($N);
      commit $It;
    end loop;
  end;
};`, "")
	})
}
