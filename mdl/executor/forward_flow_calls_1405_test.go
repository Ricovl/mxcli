// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1405: describe emits microflows in catalog order, and check
// and exec refused a call to a microflow the script creates further down — so
// mutually recursive microflows could not be re-created from describe output
// at all ("microflow ModA.MF_A references microflow ModA.MF_B before it is
// created (statement 3), and the project does not have it").
//
// A call is stored by name, so it can be written before its callee exists.
// What made it unsafe was a later statement failing and leaving the call
// dangling; a run under the all-or-nothing journal (#1408) cannot do that, so
// such a run resolves a call against every flow the script declares. A run
// that cannot roll back (--continue-on-error, the REPL, -c) still refuses it.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mutualRecursion is the reporter's describe output re-run into a project that
// has neither flow: the caller first, its callee second, each calling the other.
const mutualRecursion = `mdl 1;
create module ModR;
create or modify microflow ModR.MF_A () returns Boolean as $r
begin
  $r = call microflow ModR.MF_B();
  return $r;
end;
create or modify microflow ModR.MF_B () returns Boolean as $r
begin
  $r = call microflow ModR.MF_A();
  return $r;
end;`

func TestForwardFlowCall_ResolvedWhenTheRunCanRollBack(t *testing.T) {
	for name, src := range map[string]string{
		"create or modify, the describe output": mutualRecursion,
		"plain create":                          strings.ReplaceAll(mutualRecursion, "create or modify microflow", "create microflow"),
		"nanoflows":                             strings.ReplaceAll(mutualRecursion, "microflow", "nanoflow"),
	} {
		t.Run(name, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			exec.SetForwardFlowCalls(true)
			assertAgree(t, exec, dir, src, "")

			kind := ast.DescribeMicroflow
			if strings.Contains(src, "nanoflow") {
				kind = ast.DescribeNanoflow
			}
			out.Reset()
			assertNoError(t, exec.Execute(&ast.DescribeStmt{ObjectType: kind, Name: ast.QualifiedName{Module: "ModR", Name: "MF_A"}}))
			if !strings.Contains(out.String(), "ModR.MF_B()") {
				t.Errorf("MF_A does not call MF_B:\n%s", out.String())
			}
		})
	}
}

func TestForwardFlowCall_RefusedWhenTheRunCannotRollBack(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	exec.SetForwardFlowCalls(false)
	assertAgree(t, exec, dir, mutualRecursion, "cannot be rolled back")

	// Control: the stub-then-modify form works in every mode.
	exec2, _, dir2 := openPedAppCopy(t)
	exec2.SetForwardFlowCalls(false)
	assertAgree(t, exec2, dir2, `mdl 1;
create module ModR;
create microflow ModR.MF_B () returns Boolean as $r
begin
  return true;
end;
create microflow ModR.MF_A () returns Boolean as $r
begin
  $r = call microflow ModR.MF_B();
  return $r;
end;
create or modify microflow ModR.MF_B () returns Boolean as $r
begin
  $r = call microflow ModR.MF_A();
  return $r;
end;`, "")
}

// The exemption is the flow call alone: a page whose button calls a microflow
// created later still fails, because the page builder resolves the flow's
// parameters when it writes the page (#1212).
func TestForwardFlowCall_PageReferenceStillOrdered(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	exec.SetForwardFlowCalls(true)
	assertAgree(t, exec, dir, `mdl 1;
create module ModO;
create persistent entity ModO.Item (Name: String(100));
create page ModO.ItemPage (
  Title: 'Item',
  Layout: Atlas_Core.PopupLayout,
  Params: ( $Item: ModO.Item )
) {
  dataview dvItem (DataSource: $Item) {
    actionbutton btnReopen (Caption: 'Reopen', Action: call microflow ModO.ACT_OpenItem(Item = $Item))
  }
};
create microflow ModO.ACT_OpenItem ($Item: ModO.Item)
begin
  show page ModO.ItemPage(Item = $Item);
end;`, "ModO.ACT_OpenItem")
}
