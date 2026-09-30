// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// unitWriteDeferrer is the optional backend capability a run of access-rule
// statements is written through. A backend without it (the MCP backend, the
// mock) writes each statement as it always has.
type unitWriteDeferrer interface {
	DeferUnitWrites()
	FlushDeferredWrites() error
}

// accessRuleRun writes a run of consecutive entity access-rule statements once,
// at its end, so it is judged by the rules it leaves rather than by each step.
//
// A "reset, then authoritative grants" section — `revoke all on entity E from R;`
// followed by the grants meant to hold — rewrote the domain model on every run
// and re-minted the rule, although it ended with the rules it began with: the
// revoke was written, and the grant was reconciled against a unit that no longer
// had the rule to carry its identity from (ako/mxcli#872, rehearsal W3). Held
// until the run ends, the domain model is reconciled once against what was stored
// before the run: a run that nets to nothing writes nothing, and one that changes
// a rule keeps the $IDs of the rules it re-grants.
//
// The run is only GRANT/REVOKE on an entity. They write the domain model through
// one path and read it back through the reader, which serves the held bytes; any
// other statement ends the run first, so nothing that writes another way ever
// sees — or is shadowed by — a held unit.
type accessRuleRun struct {
	open unitWriteDeferrer
}

func isAccessRuleStmt(stmt ast.Statement) bool {
	switch stmt.(type) {
	case *ast.GrantEntityAccessStmt, *ast.RevokeEntityAccessStmt:
		return true
	}
	return false
}

// step opens the run before an access-rule statement and ends it before any
// other statement.
func (r *accessRuleRun) step(e *Executor, stmt ast.Statement) error {
	if !isAccessRuleStmt(stmt) {
		return r.end()
	}
	if r.open != nil || e.backend == nil || !e.backend.IsConnected() {
		return nil
	}
	if d, ok := e.backend.(unitWriteDeferrer); ok {
		d.DeferUnitWrites()
		r.open = d
	}
	return nil
}

// end writes what the run holds. Safe to call with no run open.
func (r *accessRuleRun) end() error {
	if r.open == nil {
		return nil
	}
	d := r.open
	r.open = nil
	if err := d.FlushDeferredWrites(); err != nil {
		return mdlerrors.NewBackend("write access rules", err)
	}
	return nil
}
