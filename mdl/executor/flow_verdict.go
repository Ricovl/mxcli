// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// # One verdict for a `create or modify` of a stored flow
//
// exec, diff and check each answer "what happens to this statement?" for a
// `create or modify microflow|nanoflow` of a flow that is stored, and they must
// give the same answer (ako/mxcli#839, ako/mxcli#876). diff once said
// "unchanged" for a statement exec refused; check said nothing at all for
// statements exec refused under mdl 1, so a corpus migrated with
// `fmt --upgrade --header` checked clean and then stopped executing.
//
// decideFlowModify is that answer, computed once; exec acts on it
// (modifyFlowInPlace), diff reports it (spliceVerdict) and check reports it
// (checkFlowModify, cmd_check_flow_verdict.go). The wording of a refusal and
// of the mdl 0 rebuild warning is shared too, so check quotes exec.

// flowVerdict is what exec does with a `create or modify` of a flow.
// Exactly one of the cases holds:
//
//   - plan != nil: the change is spliced into the stored flow (an empty patch
//     writes nothing);
//   - why != nil: the splice cannot make it, and exec refuses the statement
//     under mdl 1 or rebuilds the whole flow under mdl 0 (refused says which);
//   - err != nil: the statement fails on its own account, under every
//     version;
//   - none of them: there is no such flow (or module) yet, so it is a create.
type flowVerdict struct {
	plan *flowPlan
	why  *notSpliceable
	err  error
	// refused is why != nil under a language version that refuses the
	// rebuild (flowRebuildRefused).
	refused bool
}

// decideFlowModify works out the verdict for d in the script's language
// (ctx.LanguageVersion), writing nothing.
func decideFlowModify(ctx *ExecContext, d *flowDecl) flowVerdict {
	p, err := planFlowModify(ctx, d)
	var why *notSpliceable
	switch {
	case errors.As(err, &why):
		return flowVerdict{why: why, refused: flowRebuildRefused.Applies(ctx.LanguageVersion)}
	case err != nil:
		return flowVerdict{err: err}
	}
	return flowVerdict{plan: p}
}

// flowRefusal is exec's refusal of a change the splice cannot make (mdl 1).
func flowRefusal(d *flowDecl, why error) error {
	var ns *notSpliceable
	if errors.As(why, &ns) && ns.loopHandle != "" {
		// alter cannot splice inside a loop either, so "change activities
		// with alter" would only lead to alter's own refusal: name the one
		// alter that makes the change.
		return mdlerrors.NewValidation(fmt.Sprintf(
			"create or modify %s %s: this change cannot be spliced into the stored flow: %v. "+
				"Nothing was written: rebuilding the whole flow instead would reset what Studio Pro drew "+
				"(curves, merges, element IDs). %s",
			d.kind(), d.name, why, replaceLoopAdvice(d.kind(), d.name.String(), ns.loopHandle)))
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"create or modify %s %s: this change cannot be spliced into the stored flow: %v. "+
			"Nothing was written: rebuilding the whole flow instead would reset what Studio Pro drew "+
			"(curves, merges, element IDs). Change activities with `alter %s %s { … }`; "+
			"to rebuild the flow deliberately, drop the %s and create it",
		d.kind(), d.name, why, d.kind(), d.name, d.kind()))
}

// replaceLoopAdvice is what `create or modify` and `alter` both say about a
// change inside a loop body: neither splices inside a loop, and each used to
// send the reader to the other. What makes the change is an alter that
// replaces the whole loop — the rest of the flow keeps its element IDs and
// layout (measured on an 11.14 app: every object and flow outside the loop
// kept its $ID and position), and the loop and its body are rebuilt, which
// the statement states.
// loopHandle is the stored loop's handle as `describe … with handles` prints
// it (`loop $It in $Items`, `while $N < 10`).
func replaceLoopAdvice(kind, name, loopHandle string) string {
	end := "end loop;"
	if strings.HasPrefix(loopHandle, "while ") {
		end = "end while;"
	}
	return fmt.Sprintf("Replace the whole loop instead, stating its body as it should be "+
		"(the rest of the flow keeps its element IDs and layout; the loop and its body are rebuilt): "+
		"`alter %s %s { replace %s with begin %s begin … %s end; };`",
		kind, name, loopHandle, loopHandle, end)
}

// flowRebuildWarning is exec's MDL-V1-REBUILD warning for the whole-flow
// rebuild it runs instead under mdl 0, without the "Warning [code]: " prefix.
func flowRebuildWarning(ctx *ExecContext, d *flowDecl, why error) string {
	return fmt.Sprintf("%s %s is rebuilt as a whole: %v. %s",
		d.kind(), d.name, why, flowRebuildRefused.Warning(ctx.LanguageVersion))
}
