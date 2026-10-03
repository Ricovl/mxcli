// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

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
//
// The run's statements report only when it ends, because only then is it known
// whether they wrote: a grant that is already in force reported "Granted access
// …" on every re-run although nothing reached disk (ako/mxcli#890). Their
// reports are held and printed at the flush — as written, or as Unchanged when
// the flush offered the domain model and storage elided it.
type accessRuleRun struct {
	open unitWriteDeferrer
	e    *Executor
	held []heldReport
}

// heldReport is one statement's report in an open access-rule run.
type heldReport struct {
	// text is what the statement prints when the run wrote.
	text string
	// unchanged is the "Unchanged …" subject when the run wrote nothing; empty
	// drops the text instead (a "Reconciled N rules" line about a rewrite that
	// did not happen).
	unchanged string
	// notice is printed whatever the run wrote: it reports no write ("…
	// nothing to revoke"), so there is nothing to downgrade.
	notice bool
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
		r.open, r.e, r.held = d, e, nil
		e.accessRun = r
	}
	return nil
}

// hold records a report of a statement in the run, to be printed at its end.
func (r *accessRuleRun) hold(h heldReport) { r.held = append(r.held, h) }

// end writes what the run holds. Safe to call with no run open.
func (r *accessRuleRun) end() error {
	if r.open == nil {
		return nil
	}
	d, e, held := r.open, r.e, r.held
	r.open, r.e, r.held = nil, nil, nil
	if e.accessRun == r {
		e.accessRun = nil
	}
	before := currentWriteStats(e.backend)
	err := d.FlushDeferredWrites()
	after := currentWriteStats(e.backend)
	// The same evidence rule as ReportMutation: offered, and none landed.
	elided := err == nil && after.Offered > before.Offered && after.Written == before.Written
	for _, h := range held {
		switch {
		case !elided || h.notice:
			fmt.Fprint(e.output, h.text)
		case h.unchanged != "":
			line := fmt.Sprintf("Unchanged %s\n", h.unchanged)
			if !e.tally.countUnchanged(line) {
				fmt.Fprint(e.output, line)
			}
		}
	}
	if err != nil {
		return mdlerrors.NewBackend("write access rules", err)
	}
	return nil
}

// reportAccessRule reports an access-rule statement: held until the run it is
// part of ends, or — outside a run — judged at once like any other write.
func (ctx *ExecContext) reportAccessRule(h heldReport) {
	if ctx.accessRun != nil {
		ctx.accessRun.hold(h)
		return
	}
	switch {
	case h.notice:
		fmt.Fprint(ctx.Output, h.text)
	case ctx.mutationWasElided():
		if h.unchanged != "" {
			reportUnchanged(ctx, h.unchanged)
		}
	default:
		fmt.Fprint(ctx.Output, h.text)
	}
}
