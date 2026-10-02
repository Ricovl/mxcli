// SPDX-License-Identifier: Apache-2.0

package ast

// CreateGuard is embedded in every document-level CREATE statement that accepts
// `if not exists`. The guard is the "leave it alone" operation of ADR-0010 R1:
// when the named element already exists, the statement is skipped and the
// stored element is not touched; otherwise it creates exactly as a plain
// `create` would. It is deliberately not `create or modify`, which makes the
// stored element match the statement.
//
// The guard is honoured once, in the executor's dispatch, rather than in each
// handler — the same choice DropGuard made for `drop … if exists`: a guard that
// has to be remembered per handler is the one the next doctype forgets.
type CreateGuard struct {
	// IfNotExists is `create <kind> if not exists <name>`.
	IfNotExists bool
	// GuardWithOrModify records that the same statement also said
	// `create or modify` (or its alias `or replace`). The two guards contradict
	// each other, which check reports as MDL085.
	GuardWithOrModify bool
}

// CreateIfNotExists reports whether the statement was written with
// `if not exists`.
func (g *CreateGuard) CreateIfNotExists() bool { return g.IfNotExists }

// SetCreateIfNotExists records the guard; the visitor calls it once for
// whichever create statement it built. orModify is whether the statement also
// carried `or modify` / `or replace`.
func (g *CreateGuard) SetCreateIfNotExists(orModify bool) {
	g.IfNotExists = true
	g.GuardWithOrModify = orModify
}

// CreateGuardContradicts reports `create or modify … if not exists`.
func (g *CreateGuard) CreateGuardContradicts() bool { return g.IfNotExists && g.GuardWithOrModify }

// IfNotExistsCreate is implemented by every statement that embeds CreateGuard.
type IfNotExistsCreate interface {
	Statement
	CreateIfNotExists() bool
	SetCreateIfNotExists(orModify bool)
	CreateGuardContradicts() bool
}
