// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// The EXPORT LEVEL clause on an entity, attribute, association and enumeration
// (mendixlabs/mxcli#1498). It is the microflow clause and means what it means
// there: whether the element is usable outside a Protected (add-on) module. In an
// ordinary module mxbuild ignores it, as it does a microflow's.

// authoredExportLevel is the level a clause stated, or "" when the statement had
// none — which every writer reads as "keep what is stored" (Hidden on a new
// element).
func authoredExportLevel(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// exportLevelKeyword is the MDL spelling of a stored level: `api` or `hidden`.
func exportLevelKeyword(level string) string {
	return strings.ToLower(level)
}

// exportLevelClause is what describe prints after a domain-model element: the
// clause for a Usable one, nothing for Hidden — the default an absent clause
// writes on create, the way describe treats a microflow's level.
func exportLevelClause(level string) string {
	if level == types.ExportLevelAPI {
		return "export level api"
	}
	return ""
}
