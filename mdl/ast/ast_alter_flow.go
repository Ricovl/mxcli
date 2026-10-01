// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// ALTER MICROFLOW / ALTER NANOFLOW — a graph splice into the stored flow
// ============================================================================

// AlterFlowStmt represents:
//
//	alter microflow|nanoflow Module.Name {
//	  insert after|before <target> { <statements> }
//	  replace <target> with { <statements> }
//	  drop <target>;
//	}
//
// (ADR-0012 decision 3). Targets are content addresses, resolved against the
// flow as stored before any operation applies.
type AlterFlowStmt struct {
	Nanoflow   bool
	Name       QualifiedName
	Operations []*AlterFlowOperation
}

func (s *AlterFlowStmt) isStatement() {}

// Kind is "microflow" or "nanoflow".
func (s *AlterFlowStmt) Kind() string {
	if s.Nanoflow {
		return "nanoflow"
	}
	return "microflow"
}

// AlterFlowOpKind names an operation of an AlterFlowStmt.
type AlterFlowOpKind string

const (
	AlterFlowInsertAfter  AlterFlowOpKind = "insert after"
	AlterFlowInsertBefore AlterFlowOpKind = "insert before"
	AlterFlowReplace      AlterFlowOpKind = "replace"
	AlterFlowDrop         AlterFlowOpKind = "drop"
)

// AlterFlowOperation is one operation of an AlterFlowStmt.
type AlterFlowOperation struct {
	Op AlterFlowOpKind
	// Target is the content address as written (`$IsValidEmail`,
	// `'Email is Valid?'`, `log * node 'Debug' *`, with an optional `@n`).
	// mfmutator.ParseTarget reads it.
	Target string
	// Body is the fragment, for insert and replace.
	Body []MicroflowStatement
	// ReplaceNotes, on a replace or drop, says the notes attached to the
	// target go with it instead of being kept on the replacement (or left
	// behind): Body carries the notes the statement states. Set only by
	// `create or modify`, whose declared statements state every activity's
	// notes (ako/mxcli#859); an `alter` replace keeps them.
	ReplaceNotes bool
	// SetCondition, on a replace of a decision, says Body is a single `if`
	// whose condition is the decision's new one, set in place: the decision,
	// its flows and both of its paths stay (ako/mxcli#888). Set only by
	// `create or modify`, for an `if` that differs from the stored one in its
	// condition; the branches are diffed by operations of their own.
	SetCondition bool
}
