// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCommitStatement records a `commit $X` written in the body of a
// `create or modify microflow|nanoflow` (ast.Program.FlowCommits) for
// `fmt --upgrade -p` (ako/mxcli#873).
//
// Since #895 a bare `commit $X;` means WITH events, Studio Pro's default; an
// older mxcli stored the same statement without events. Re-running such a
// script therefore changes what the stored flow does, and inside a loop the
// mdl 1 splice refuses it outright. Which of the two a bare commit should be
// is in the stored flow, not in the script, so the visitor only records where
// each commit is and how to spell the stored flag; the upgrade asks the
// project.
//
// A commit in an `alter microflow` fragment or a plain `create` is not
// recorded: it is new code, which the current default describes.
func (b *Builder) ExitCommitStatement(ctx *parser.CommitStatementContext) {
	v := ctx.VARIABLE()
	if v == nil {
		return
	}
	flow, nanoflow, ok := enclosingModifiedFlow(ctx)
	if !ok {
		return
	}
	c := ast.FlowCommit{
		Line:          ctx.GetStart().GetLine(),
		Flow:          flow,
		Nanoflow:      nanoflow,
		Variable:      strings.TrimPrefix(v.GetText(), "$"),
		Bare:          ctx.WITH() == nil && ctx.WITHOUT() == nil,
		WithoutEvents: ctx.WITHOUT() != nil,
	}
	if c.Bare {
		text := " without events"
		if kw := ctx.COMMIT().GetText(); kw == strings.ToUpper(kw) {
			text = strings.ToUpper(text)
		}
		at := v.GetSymbol().GetStop() + 1
		c.PinWithoutEvents = &ast.Fix{Edits: []ast.TextEdit{{Start: at, Stop: at, Text: text}}}
	}
	b.flowCommits = append(b.flowCommits, c)
}

// enclosingModifiedFlow names the flow a statement is written in when that is
// a `create or modify` (or its alias `create or replace`) microflow or
// nanoflow.
func enclosingModifiedFlow(ctx antlr.RuleContext) (name ast.QualifiedName, nanoflow, ok bool) {
	for p := ctx.GetParent(); p != nil; p = p.GetParent() {
		var qn parser.IQualifiedNameContext
		switch f := p.(type) {
		case *parser.CreateMicroflowStatementContext:
			qn = f.QualifiedName()
		case *parser.CreateNanoflowStatementContext:
			qn, nanoflow = f.QualifiedName(), true
		default:
			continue
		}
		cs := findParentCreateStatement(p.(antlr.RuleContext))
		if qn == nil || cs == nil || cs.OR() == nil || (cs.MODIFY() == nil && cs.REPLACE() == nil) {
			return ast.QualifiedName{}, false, false
		}
		return buildQualifiedName(qn), nanoflow, true
	}
	return ast.QualifiedName{}, false, false
}
