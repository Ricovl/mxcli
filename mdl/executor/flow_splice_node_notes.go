// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
)

// setNodeNotes gives the stored decision or end event target the notes op's
// statement states (mendixlabs/mxcli#1517): the stored notes are taken off,
// and the stated ones are built against a stand-in activity — so they are
// drawn as a full build would draw them for the node — and attached to target.
func (a *alterFlowContext) setNodeNotes(ctx *ExecContext, mut backend.MicroflowMutator, op *ast.AlterFlowOperation, target mfmutator.Candidate) error {
	if err := mut.RemoveNotes(target.ID); err != nil {
		return err
	}
	notes := statementNotes(op.Body[0])
	if len(notes) == 0 {
		return nil
	}
	standIn := &ast.LogStmt{
		Level:       ast.LogInfo,
		Message:     &ast.LiteralExpr{Value: "", Kind: ast.LiteralString},
		Annotations: &ast.ActivityAnnotations{Notes: notes},
	}
	frag, err := a.buildFragment(ctx, []ast.MicroflowStatement{standIn})
	if err != nil {
		return err
	}
	return mut.AddNotes(target.ID, frag)
}

// setFreeNotes replaces the stored flow's free notes by notes
// (mendixlabs/mxcli#1517), built as a full build draws them: a stand-in
// activity carries them as free notes, and the mutator keeps the notes alone.
func (a *alterFlowContext) setFreeNotes(ctx *ExecContext, mut backend.MicroflowMutator, notes []ast.MicroflowAnnotation) error {
	if len(notes) == 0 {
		return mut.SetFreeNotes(nil)
	}
	standIn := &ast.LogStmt{
		Level:       ast.LogInfo,
		Message:     &ast.LiteralExpr{Value: "", Kind: ast.LiteralString},
		Annotations: &ast.ActivityAnnotations{FreeNotes: notes},
	}
	frag, err := a.buildFragment(ctx, []ast.MicroflowStatement{standIn})
	if err != nil {
		return err
	}
	return mut.SetFreeNotes(frag)
}
