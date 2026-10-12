// SPDX-License-Identifier: Apache-2.0

package executor

// What is in scope after `merge <label>` comes from the paths that `join` it,
// not from the text above it.
//
// A description of an irreducible graph prints each shared region once, in a
// section after the branches that reach it (emitCrossedMergeSections). Read in
// text order, a variable those branches declared is used "before" it exists,
// and check rejected the faithful description of a valid microflow with
// "variable 'IsValid' is not declared" (Evora AgentCommons.Tool_Validate: the
// variable is declared inside a `split type` arm, and the arm joins the
// section that sets it).

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

const mergeSectionScopeMDL = `create microflow M.SectionScope ($E: M.Kind) returns Boolean
begin
  case $E
    when One then
      declare $IsValid Boolean = true;
      join checks;
    when Two then
      return false;
  end case;
  merge checks;
  set $IsValid = false;
  return $IsValid;
end;`

func createMicroflowStmtOf(t *testing.T, src string) *ast.CreateMicroflowStmt {
	t.Helper()
	for _, stmt := range parseMDL(t, src).Statements {
		if c, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			return c
		}
	}
	t.Fatal("no CREATE MICROFLOW in the parsed program")
	return nil
}

// mendixlabs/mxcli#1436: `returns Boolean as $Closed` names the result for
// callers and declares nothing, so `set $Closed = true` is correctly refused —
// but "variable 'Closed' is not declared" read as contradicting the very
// clause that names it. The message must say what `as $Closed` does.
func TestValidateFlowBody_SetOnReturnsAsNameExplainsTheClause(t *testing.T) {
	stmt := createMicroflowStmtOf(t, "create microflow M.F () returns Boolean as $Closed\nbegin\n  set $Closed = true;\n  return $Closed;\nend;")
	errs := ValidateMicroflowBody(stmt)
	if len(errs) != 1 {
		t.Fatalf("want exactly one error (semantics unchanged), got %v", errs)
	}
	for _, want := range []string{"is not declared", "returns … as $Closed", "declare $Closed Boolean"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should contain %q, got: %s", want, errs[0])
		}
	}
}

// Control: another undeclared variable keeps the generic message, with no
// mention of the `as` clause.
func TestValidateFlowBody_SetOnOtherUndeclaredNameKeepsGenericMessage(t *testing.T) {
	stmt := createMicroflowStmtOf(t, "create microflow M.F () returns Boolean as $Closed\nbegin\n  set $Other = true;\n  return true;\nend;")
	errs := ValidateMicroflowBody(stmt)
	if len(errs) != 1 || strings.Contains(errs[0], "returns …") {
		t.Fatalf("want the generic not-declared error, got %v", errs)
	}
}

func TestValidateFlowBody_MergeSeesItsJoinsVariables(t *testing.T) {
	stmt := createMicroflowStmtOf(t, mergeSectionScopeMDL)
	if errs := ValidateMicroflowBody(stmt); len(errs) > 0 {
		t.Errorf("a variable declared on the path that joins the merge was reported: %v", errs)
	}
}

// The control: the scope comes from the JOIN, not from anything after it.
// Without a join from the declaring branch the variable is still undeclared.
func TestValidateFlowBody_MergeWithoutTheDeclaringJoinStillReports(t *testing.T) {
	src := strings.Replace(mergeSectionScopeMDL, "      join checks;\n", "      return true;\n", 1)
	src = strings.Replace(src, "      return false;\n", "      join checks;\n", 1)
	stmt := createMicroflowStmtOf(t, src)
	errs := ValidateMicroflowBody(stmt)
	if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "IsValid") {
		t.Errorf("a merge reached only from a branch that never declared $IsValid must still report it, got %v", errs)
	}
}

// MDL005 is the linear "declared in a branch, used after it" warning. Past a
// merge it cannot see the joins, so it must not guess.
func TestValidateMicroflow_NoBranchScopeWarningPastAMerge(t *testing.T) {
	stmt := createMicroflowStmtOf(t, mergeSectionScopeMDL)
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL005" {
			t.Errorf("MDL005 fired past a merge: %s", v.Message)
		}
	}
}
