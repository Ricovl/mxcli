// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A change of a loop body's shape the splice cannot make — a decision added
// in it — is refused by both editing modes: `create or modify` under mdl 1
// and `alter`. Each refusal used to send the reader to the other — create or
// modify said "change activities with alter", alter said "rewrite the loop
// with create or modify" — and an alter of an activity inside the loop that
// reads the iterator failed first on "the fragment uses $It, which is not
// declared on the path", before the real reason. Both refusals now name the
// statement that makes the change, `replace body of` the loop, by the stored
// loop's own handle (ako/mxcli#1693; replacing the whole loop, which they
// named before, re-minted every identity in it).

const loopAdviceFlow = `mdl 1;
create microflow MyFirstModule.LoopAdvice ($Items: List of FeedbackModule.Feedback)
begin
  log info node 'X' 'start';
  loop $It in $Items
  begin
    commit $It;
  end loop;
  log info node 'X' 'done';
end;`

// loopAdviceUnit returns LoopAdvice's stored bytes.
func loopAdviceUnit(t *testing.T, exec *Executor) []byte {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == "LoopAdvice" && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "MyFirstModule" {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte(nil), raw...)
		}
	}
	t.Fatal("MyFirstModule.LoopAdvice not found")
	return nil
}

// The advice both refusals give: the alter that replaces the stored loop's
// body, addressed by its handle.
const loopAdviceReplace = "replace body of loop $It in $Items with begin"

// loopAdviceDecision is a decision for the loop body, which the splice does
// not put in one.
const loopAdviceDecision = "if $It/Subject = empty then log info node 'X' 'no subject'; end if;"

// checkSays asserts `check -p`'s flow verdict for src carries want.
func checkSays(t *testing.T, exec *Executor, src, want string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var says []string
	for _, v := range exec.CheckFlowVerdicts(prog) {
		says = append(says, v.Message)
	}
	if !strings.Contains(strings.Join(says, "\n"), want) {
		t.Errorf("check -p must predict the refusal with %q, reported: %v", want, says)
	}
}

func TestFlowLoopRefusals_PointAtReplaceBody(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, loopAdviceFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	stored := loopAdviceUnit(t, exec)

	t.Run("create or modify names alter replace body", func(t *testing.T) {
		src := strings.Replace(loopAdviceFlow, "create microflow", "create or modify microflow", 1)
		// A decision inside the body: a change of the body's shape the
		// splice does not make (a plain activity it splices, ako/mxcli#1693).
		src = strings.Replace(src, "commit $It;", "commit $It;\n    "+loopAdviceDecision, 1)
		// check predicts exec's refusal, with the same advice.
		checkSays(t, exec, src, loopAdviceReplace)
		err := agreeExec(t, exec, src)
		if err == nil {
			t.Fatal("want the in-loop change refused under mdl 1")
		}
		msg := err.Error()
		if !strings.Contains(msg, "alter microflow MyFirstModule.LoopAdvice {") || !strings.Contains(msg, loopAdviceReplace) {
			t.Errorf("the refusal does not spell out the alter to run: %s", msg)
		}
		if !strings.Contains(msg, "a decision is added to it") {
			t.Errorf("the refusal does not say what is out of reach: %s", msg)
		}
		// mendixlabs/mxcli#1517: the deliberate rebuild is named too, as
		// every other splice refusal names it.
		if !strings.Contains(msg, "drop the microflow and create it in the same script") {
			t.Errorf("the refusal does not name the drop-and-recreate route: %s", msg)
		}
		// The generic advice is what sent the reader into alter's own
		// in-loop refusal.
		if strings.Contains(msg, "Change activities with") {
			t.Errorf("the refusal still gives the generic alter advice: %s", msg)
		}
		if !bytes.Equal(stored, loopAdviceUnit(t, exec)) {
			t.Error("the refused statement wrote")
		}
	})

	t.Run("alter of an activity in the loop names replace body, not scope", func(t *testing.T) {
		for _, op := range []string{
			"replace commit $It with begin " + loopAdviceDecision + " end;",
			"insert after commit $It begin " + loopAdviceDecision + " end;",
		} {
			src := "mdl 1;\nalter microflow MyFirstModule.LoopAdvice {\n  " + op + "\n};"
			checkSays(t, exec, src, loopAdviceReplace)
			err := agreeExec(t, exec, src)
			if err == nil {
				t.Fatalf("%s: want an in-loop refusal", op)
			}
			msg := err.Error()
			if strings.Contains(msg, "not declared on the path") {
				t.Errorf("%s: the loop iterator is reported out of scope before the real reason: %s", op, msg)
			}
			if !strings.Contains(msg, "inside the body of loop $It in $Items") || !strings.Contains(msg, loopAdviceReplace) ||
				!strings.Contains(msg, "the fragment holds a decision") {
				t.Errorf("%s: want the in-loop reason and the replace-body advice, got: %s", op, msg)
			}
			if strings.Contains(msg, "create or modify") {
				t.Errorf("%s: alter's refusal sends the reader back to create or modify: %s", op, msg)
			}
			if !bytes.Equal(stored, loopAdviceUnit(t, exec)) {
				t.Errorf("%s: the refused alter wrote", op)
			}
		}
	})

	// In a nested loop the loop named is the one whose body the change is
	// in: replace body addresses any loop, a nested one included.
	t.Run("nested loop names the inner loop", func(t *testing.T) {
		if err := agreeExec(t, exec, `mdl 1;
create microflow MyFirstModule.LoopAdviceNested ($Items: List of FeedbackModule.Feedback, $Others: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    loop $Other in $Others
    begin
      commit $Other;
    end loop;
  end loop;
end;`); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := agreeExec(t, exec, "mdl 1;\nalter microflow MyFirstModule.LoopAdviceNested {\n"+
			"  insert after commit $Other begin if $Other/Subject = empty then log info node 'X' 'none'; end if; end;\n};")
		if err == nil || !strings.Contains(err.Error(), "inside the body of loop $Other in $Others") ||
			!strings.Contains(err.Error(), "replace body of loop $Other in $Others with begin") {
			t.Fatalf("want the inner loop named, got %v", err)
		}
	})

	// Control: a fragment at top level that reads a variable not on its path
	// is still refused by the scope check, with its own message.
	t.Run("control: scope check outside a loop", func(t *testing.T) {
		err := agreeExec(t, exec, "mdl 1;\nalter microflow MyFirstModule.LoopAdvice {\n"+
			"  insert after log node 'X' 'start' begin commit $Nope; end;\n};")
		if err == nil || !strings.Contains(err.Error(), "not declared on the path") {
			t.Fatalf("want the scope refusal, got %v", err)
		}
	})

	// One activity replaced by one of the same kind is made in place, with
	// the loop's iterator in scope (ako/mxcli#1106); run last but one, as it
	// writes.
	t.Run("alter replaces one activity in the loop in place", func(t *testing.T) {
		err := agreeExec(t, exec, "mdl 1;\nalter microflow MyFirstModule.LoopAdvice {\n"+
			"  replace commit $It with begin commit $It without events; end;\n};")
		if err != nil {
			t.Fatalf("an in-place replace inside the loop: %v", err)
		}
		var buf bytes.Buffer
		exec.output = &buf
		if err := afRun(t, exec, "describe microflow MyFirstModule.LoopAdvice;"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "commit $It without events;") {
			t.Errorf("the alter did not write the change:\n%s", buf.String())
		}
	})

	// Control: the advice works — the replace-body alter is accepted and
	// writes the decision into the stored loop.
	t.Run("control: the advised alter applies", func(t *testing.T) {
		err := agreeExec(t, exec, `mdl 1;
alter microflow MyFirstModule.LoopAdvice {
  replace body of loop $It in $Items with begin
    commit $It without events;
    `+loopAdviceDecision+`
  end;
};`)
		if err != nil {
			t.Fatalf("the advised alter: %v", err)
		}
		var buf bytes.Buffer
		exec.output = &buf
		if err := afRun(t, exec, "describe microflow MyFirstModule.LoopAdvice;"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "commit $It without events;") || !strings.Contains(buf.String(), "if $It/Subject = empty then") {
			t.Errorf("the alter did not write the change:\n%s", buf.String())
		}
	})
}
