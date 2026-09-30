// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// exitAlterFlowStatement builds an AlterFlowStmt from
// ALTER MICROFLOW|NANOFLOW Module.Name { insert / replace / drop }.
func (b *Builder) exitAlterFlowStatement(ctx *parser.AlterStatementContext) {
	stmt := &ast.AlterFlowStmt{Nanoflow: ctx.NANOFLOW() != nil}
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Name = buildQualifiedName(qn)
	}
	for _, opCtx := range ctx.AllAlterFlowOperation() {
		op := opCtx.(*parser.AlterFlowOperationContext)
		o := &ast.AlterFlowOperation{Target: alterFlowTargetText(op.AlterFlowTarget())}
		switch {
		case op.INSERT() != nil && op.BEFORE() != nil:
			o.Op = ast.AlterFlowInsertBefore
		case op.INSERT() != nil:
			o.Op = ast.AlterFlowInsertAfter
		case op.REPLACE() != nil:
			o.Op = ast.AlterFlowReplace
		default:
			o.Op = ast.AlterFlowDrop
		}
		if frag, ok := op.AlterFlowFragment().(*parser.AlterFlowFragmentContext); ok && frag != nil {
			if body := frag.MicroflowBody(); body != nil {
				o.Body = buildMicroflowBody(body)
			}
		}
		stmt.Operations = append(stmt.Operations, o)
	}
	b.statements = append(b.statements, stmt)
}

// alterFlowTargetText returns a target as the author wrote it — whitespace and
// quoting included — since a statement pattern is matched token by token
// against describe's rendering, and the grammar only knows it as a run of
// arbitrary tokens.
func alterFlowTargetText(ctx parser.IAlterFlowTargetContext) string {
	if ctx == nil {
		return ""
	}
	start, stop := ctx.GetStart(), ctx.GetStop()
	if start == nil || stop == nil {
		return strings.TrimSpace(ctx.GetText())
	}
	return strings.TrimSpace(start.GetInputStream().GetText(start.GetStart(), stop.GetStop()))
}
