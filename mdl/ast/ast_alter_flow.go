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
}
