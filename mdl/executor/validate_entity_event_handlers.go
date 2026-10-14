// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateEntityEventHandlers resolves the microflow each entity event handler
// calls (mendixlabs/mxcli#1472). exec resolves it (buildEventHandlers) and
// refuses a missing one with "microflow not found" — after the statements
// before it are written — while check never looked, so `alter entity … add
// event handler on before commit call M.Typo(…)` reported "All references
// valid". A microflow the script creates counts as found; an unqualified name,
// or a backend that lists no microflows, is not established and stays silent.
func validateEntityEventHandlers(ctx *ExecContext, sc *scriptContext, entity string, defs []ast.EventHandlerDef) error {
	if len(defs) == 0 || !ctx.Connected() {
		return nil
	}
	var known map[string]bool
	for _, d := range defs {
		qn := d.Microflow.String()
		if d.Microflow.Module == "" || d.Microflow.Name == "" || sc.microflows[qn] {
			continue
		}
		if known == nil {
			known = buildMicroflowQualifiedNames(ctx)
		}
		if len(known) == 0 {
			return nil // a backend that lists none cannot say one is missing
		}
		if !known[qn] {
			return mdlerrors.NewNotFoundMsg("microflow", qn, fmt.Sprintf(
				"entity %s: the %s %s event handler calls microflow %s, which was not found — exec refuses it",
				entity, strings.ToLower(d.Moment), strings.ToLower(d.Event), qn))
		}
	}
	return nil
}
