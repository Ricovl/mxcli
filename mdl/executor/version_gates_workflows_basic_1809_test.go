// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1809: Workflows$Workflow is a 9.0.2 type (mendixmodelsdk 4.117.0),
// and the write refuses it below that (canon.UndeclaredTypeError, #1774). The
// `workflows basic` row said 9.0.0, and only `alter workflow` applied it, so
// on 9.0.0 or 9.0.1 `create workflow` passed `check -p` and was refused part-way
// through exec. The row is now the type's introduction, and every statement
// that writes a workflow applies it: `create workflow`, `create or modify
// workflow` and `alter workflow`. The hint names no version: the registry's
// floor is the one the message states, and the old hint's "9.12+" was not it.
func TestCheckVersionGates_WorkflowsBasic(t *testing.T) {
	name := ast.QualifiedName{Module: "M", Name: "WF"}
	for _, c := range []struct {
		stmt ast.Statement
		want string
	}{
		{&ast.CreateWorkflowStmt{Name: name}, "create workflow requires Mendix 9.0.2+"},
		{&ast.CreateWorkflowStmt{Name: name, CreateOrModify: true}, "create workflow requires Mendix 9.0.2+"},
		{&ast.AlterWorkflowStmt{Name: name}, "alter workflow requires Mendix 9.0.2+"},
	} {
		prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, c.stmt}}
		for _, below := range [][3]int{{9, 0, 0}, {9, 0, 1}} {
			vs := versionGateViolations(versionCtx(t, below[0], below[1], below[2]), prog)
			if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want) {
				t.Errorf("%T (create or modify: %v) on %v: want one MDL-VERSION01 %q, got %+v",
					c.stmt, isCreateOrModify(c.stmt), below, c.want, vs)
				continue
			}
			if strings.Contains(vs[0].Suggestion, "9.") || vs[0].Suggestion == "" {
				t.Errorf("%T: the hint must say what to do without naming a version (the message has the floor): %q",
					c.stmt, vs[0].Suggestion)
			}
			// exec's handler refuses with the same text.
			if err := checkVersionGates(versionCtx(t, below[0], below[1], below[2]), c.stmt); err == nil ||
				!strings.Contains(err.Error(), c.want) {
				t.Errorf("%T on %v: exec's handler refusal %v, want %q", c.stmt, below, err, c.want)
			}
		}
		if vs := versionGateViolations(versionCtx(t, 9, 0, 2), prog); len(vs) != 0 {
			t.Errorf("%T reported at its floor 9.0.2: %+v", c.stmt, vs)
		}
	}
}

func isCreateOrModify(stmt ast.Statement) bool {
	s, ok := stmt.(*ast.CreateWorkflowStmt)
	return ok && s.CreateOrModify
}
