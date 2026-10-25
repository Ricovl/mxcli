// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1399 (mendixlabs/mxcli#1517): the operations of one alter block —
// and the patch `create or modify` splices with, which is one — write one
// flow, but two of their variable verdicts were still taken against the flow
// as stored.
//
//   - Repro B: a fragment was BUILT knowing only the stored flow's variables,
//     so `set $Late = …` in a replaced loop failed "variable 'Late' is not
//     declared" although an earlier operation of the block declares $Late
//     upstream of it — at exec only: check gave up on the fragment ("not
//     predicted") and passed.
//   - Used return: a producer moved earlier — inserted upstream, the stored
//     one dropped — was refused "the fragment declares $Code, which the
//     microflow already has" by the stored producer the same block drops,
//     and its readers were "still used by" readers the inserted producer
//     now feeds.

const scopeMoveFlow = `mdl 1;
create microflow MyFirstModule.ScopeMove () returns String
begin
  declare $N Integer = 1;
  $Code = call microflow MyFirstModule.ScopeHelper();
  log info node 'T' $Code;
  return $Code;
end;`

func openBlockVarsFixture(t *testing.T) *Executor {
	t.Helper()
	exec := openScopeFixture(t)
	if err := agreeExec(t, exec, scopeMoveFlow); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return exec
}

// predictedAgree is scopeAgree that also requires check to have predicted the
// statement rather than given up on it: a "not predicted" note let check pass
// a statement exec then refused.
func predictedAgree(t *testing.T, exec *Executor, src, want string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	if r := exec.CheckFlowVerdictsReport(prog); len(r.Unpredicted) > 0 {
		t.Errorf("check must predict the statement, it reported it unpredicted: %+v", r.Unpredicted)
	}
	scopeAgree(t, exec, src, want)
}

// Repro B: a declaration an earlier operation of the block makes is known to
// the builder of a later operation's fragment, not only to the scope check.
func TestAlterMicroflow_FragmentBuiltWithAnEarlierOperationsDeclaration(t *testing.T) {
	t.Run("a replaced loop sets a variable an earlier insert declares", func(t *testing.T) {
		exec := openScopeFixture(t)
		predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'start' begin declare $Late Integer = 0; end;
  replace loop $It in $Items with begin
    loop $It in $Items
    begin
      set $Late = $Late + 1;
      commit $It;
    end loop;
  end;
};`, "")
		got := describeFlow(t, exec, "ScopeLoop")
		if d, s := strings.Index(got, "declare $Late Integer = 0;"), strings.Index(got, "set $Late = $Late + 1;"); d < 0 || s < d {
			t.Errorf("the declaration and the loop that sets it were not written in order:\n%s", got)
		}
	})
	t.Run("an entity variable an earlier insert retrieves is changed by a later fragment", func(t *testing.T) {
		exec := openScopeFixture(t)
		predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'start' begin retrieve $Me from System.User first; end;
  replace log node 'T' 'done' with begin change $Me (Name = 'x'); end;
};`, "")
		if got := describeFlow(t, exec, "ScopeLoop"); !strings.Contains(got, "change $Me (Name = 'x')") {
			t.Errorf("the change was not written:\n%s", got)
		}
	})
	// Control: known to the builder is not in scope. A declaration the block
	// makes downstream of the loop is still refused, by check and exec alike.
	t.Run("control: declared after the loop", func(t *testing.T) {
		exec := openScopeFixture(t)
		predictedAgree(t, exec, `mdl 1;
alter microflow MyFirstModule.ScopeLoop {
  insert after log node 'T' 'done' begin declare $Late Integer = 0; end;
  replace loop $It in $Items with begin
    loop $It in $Items
    begin
      set $Late = $Late + 1;
      commit $It;
    end loop;
  end;
};`, "not declared on the path")
	})
}

// Used return: a producer moved upstream of where it was, its readers left
// where they are.
func TestCreateOrModify_ProducerMovedEarlierKeepsItsReaders(t *testing.T) {
	exec := openBlockVarsFixture(t)
	before := flowObjectIDs(loopEditFlowOf(t, exec, "ScopeMove").ObjectCollection)
	predictedAgree(t, exec, `mdl 1;
create or modify microflow MyFirstModule.ScopeMove () returns String
begin
  $Code = call microflow MyFirstModule.ScopeHelper();
  declare $N Integer = 1;
  log info node 'T' $Code;
  return $Code;
end;`, "")
	got := describeFlow(t, exec, "ScopeMove")
	if c, n := strings.Index(got, "$Code = call microflow"), strings.Index(got, "declare $N"); c < 0 || n < c {
		t.Fatalf("the producer was not moved ahead of the declare:\n%s", got)
	}
	// The move is a new producer and the stored one gone; the declare, the
	// log and the end event keep their $IDs.
	after := flowObjectIDs(loopEditFlowOf(t, exec, "ScopeMove").ObjectCollection)
	var gone, added []model.ID
	for id := range before {
		if _, ok := after[id]; !ok {
			gone = append(gone, id)
		}
	}
	for id := range after {
		if _, ok := before[id]; !ok {
			added = append(added, id)
		}
	}
	if len(gone) != 1 || len(added) != 1 {
		t.Errorf("want the producer alone re-created (1 gone, 1 new), got %d gone, %d new", len(gone), len(added))
	}

	// Control: moved past its reader, the producer is still refused.
	exec = openBlockVarsFixture(t)
	predictedAgree(t, exec, `mdl 1;
create or modify microflow MyFirstModule.ScopeMove () returns String
begin
  declare $N Integer = 1;
  log info node 'T' $Code;
  $Code = call microflow MyFirstModule.ScopeHelper();
  return $Code;
end;`, "$Code")
}

// The same move written as an alter, in either order, and with the moved
// producer's reader in the same fragment.
func TestAlterMicroflow_ProducerMovedEarlier(t *testing.T) {
	for name, src := range map[string]string{
		"insert, then drop": `mdl 1;
alter microflow MyFirstModule.ScopeMove {
  insert before declare $N Integer = 1 begin $Code = call microflow MyFirstModule.ScopeHelper(); end;
  drop $Code = call microflow MyFirstModule.ScopeHelper();
};`,
		"drop, then insert": `mdl 1;
alter microflow MyFirstModule.ScopeMove {
  drop $Code = call microflow MyFirstModule.ScopeHelper();
  insert before declare $N Integer = 1 begin $Code = call microflow MyFirstModule.ScopeHelper(); end;
};`,
		"the producer and its reader moved together": `mdl 1;
alter microflow MyFirstModule.ScopeMove {
  insert before declare $N Integer = 1 begin
    $Code = call microflow MyFirstModule.ScopeHelper();
    log info node 'T' $Code;
  end;
  drop $Code = call microflow MyFirstModule.ScopeHelper();
  drop log node 'T' *;
};`,
	} {
		t.Run(name, func(t *testing.T) {
			exec := openBlockVarsFixture(t)
			predictedAgree(t, exec, src, "")
			got := describeFlow(t, exec, "ScopeMove")
			if c, n := strings.Index(got, "$Code = call microflow"), strings.Index(got, "declare $N"); c < 0 || n < c || strings.Count(got, "$Code = call microflow") != 1 {
				t.Errorf("want the one producer ahead of the declare:\n%s", got)
			}
		})
	}
	// Controls: the stored producer stays, so the inserted one clashes with
	// it; and a reader the block puts ahead of the new producer reads nothing.
	for name, c := range map[string]struct{ src, want string }{
		"the stored producer is kept": {`mdl 1;
alter microflow MyFirstModule.ScopeMove {
  insert before declare $N Integer = 1 begin $Code = call microflow MyFirstModule.ScopeHelper(); end;
};`, "which the microflow already has"},
		"a reader ahead of the new producer": {`mdl 1;
alter microflow MyFirstModule.ScopeMove {
  insert before declare $N Integer = 1 begin log info node 'T' $Code; end;
  insert after declare $N Integer = 1 begin $Code = call microflow MyFirstModule.ScopeHelper(); end;
  drop $Code = call microflow MyFirstModule.ScopeHelper();
};`, "$Code"},
	} {
		t.Run("control: "+name, func(t *testing.T) {
			exec := openBlockVarsFixture(t)
			predictedAgree(t, exec, c.src, c.want)
		})
	}
}
