// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1280: `show message … type error` was stored as "error".
// Microflows$ShowMessageAction.Type is an enumeration whose values are
// "Information", "Warning", "Error"; the runtime does not recognise "error" and
// falls back to an Information dialog, so an error message showed as
// information while `mx check` reported 0 errors. MDL keywords are written in
// lower case, so the type word must be matched case-insensitively and stored
// in the enumeration's own spelling.
func TestShowMessageTypeIsCanonicalised(t *testing.T) {
	cases := map[string]string{
		"error":       "Error",
		"ERROR":       "Error",
		"Error":       "Error",
		"warning":     "Warning",
		"information": "Information",
		"Information": "Information",
		`"Error"`:     "Error",
	}
	for word, want := range cases {
		t.Run(word, func(t *testing.T) {
			prog := mustBuild(t, "create microflow M.F () begin show message 'x' type "+word+"; end;")
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			got := mf.Body[0].(*ast.ShowMessageStmt).Type
			if got != want {
				t.Errorf("type %s stored as %q, want %q", word, got, want)
			}
		})
	}

	// Not an enumeration value: kept as written, so `check` can name it.
	prog := mustBuild(t, "create microflow M.F () begin show message 'x' type ok; end;")
	if got := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ShowMessageStmt).Type; got != "ok" {
		t.Errorf("unknown type stored as %q, want it kept as %q", got, "ok")
	}
}
