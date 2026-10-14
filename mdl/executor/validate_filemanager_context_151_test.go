// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func fileManagerContextErrors(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	switch s := prog.Statements[0].(type) {
	case *ast.CreatePageStmtV3:
		return fileManagersWithoutContext(s.Widgets, false)
	case *ast.CreateSnippetStmtV3:
		return fileManagersWithoutContext(s.Widgets, true)
	}
	t.Fatalf("unexpected statement %T", prog.Statements[0])
	return nil
}

// mx check, Mendix 11.14.0: a file manager in no data container at all is
// CE6810 "A file manager must be placed in a data view or snippet that is
// connected to entity 'System.FileDocument' or a specialization." Refused at
// check time, with no project needed.
func TestFileManagerContext_NoContainerIsRefused(t *testing.T) {
	got := fileManagerContextErrors(t, `create page M.P (Title: 'x', Layout: A.B) {
  container c1 { filemanager fmBare (Label: 'File') }
}`)
	if len(got) != 1 || !strings.Contains(got[0], "fmBare") || !strings.Contains(got[0], "CE6810") {
		t.Errorf("want one CE6810 error naming fmBare, got %v", got)
	}
}

// Controls, each measured clean by mx check on 11.14.0 or the shape the
// builder already guards: inside a data view, and at the top level of a
// snippet that has a parameter.
func TestFileManagerContext_ContainersAreAccepted(t *testing.T) {
	for name, src := range map[string]string{
		"data view": `create page M.P (Title: 'x', Layout: A.B, Params: ( $U: M.Upload )) {
  dataview dv (DataSource: $U) { container c { filemanager fm (Label: 'File') } }
}`,
		"snippet with a parameter": `create snippet M.S ( Params: ( $U: M.Upload ) ) {
  filemanager fm (Label: 'File')
}`,
	} {
		if got := fileManagerContextErrors(t, src); len(got) != 0 {
			t.Errorf("%s: refused: %v", name, got)
		}
	}
}

// A snippet's own object is decided where it is used — a parameter, or the
// data view the snippet call sits in — so its top level is never refused here.
func TestFileManagerContext_SnippetWithoutParameterIsAccepted(t *testing.T) {
	if got := fileManagerContextErrors(t, `create snippet M.S {
  filemanager fm (Label: 'File')
}`); len(got) != 0 {
		t.Errorf("refused: %v", got)
	}
}
