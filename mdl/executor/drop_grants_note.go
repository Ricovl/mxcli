// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// writeDroppedGrantsNote says which module-role grants a drop removed, and
// whether a later create gets them back (ako/mxcli#944).
//
// A dropped microflow or nanoflow is remembered in the session cache
// (rememberDroppedMicroflow), so a create of the same name later in the SAME
// script or REPL session carries the grants. A create in a later run starts
// from nothing — CapTrack's apply.sh dropped in one run and created in the
// next, and its pages lost access (CE0106) with only "Dropped microflow" to go
// on. A page is never remembered, so a create after its drop never carries.
//
// carries reports whether this document kind is remembered at all. grantVerb
// and kind spell the re-grant statement (`grant execute on microflow …`).
func writeDroppedGrantsNote(w io.Writer, kind, grantVerb, qualifiedName string, roles []model.ID, carries bool) {
	if len(roles) == 0 {
		return
	}
	names := strings.Join(documentRoleStrings(roles), ", ")
	if carries {
		fmt.Fprintf(w, "  Removed its %s grants to %s. A create of %s later in this script or session "+
			"carries them; a create in a later run does not — do the drop and the create in one script, "+
			"or re-grant: grant %s on %s %s to %s;\n",
			grantVerb, names, qualifiedName, grantVerb, kind, qualifiedName, names)
		return
	}
	fmt.Fprintf(w, "  Removed its %s grants to %s. A create of %s does not carry them; "+
		"re-grant: grant %s on %s %s to %s;\n",
		grantVerb, names, qualifiedName, grantVerb, kind, qualifiedName, names)
}
