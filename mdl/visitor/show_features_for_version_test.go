// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestShowFeaturesForVersionRoutesToFeatures is ako/mxcli#910: `show features
// for version 10.24` printed the session's `show version` output, because the
// bare `ctx.VERSION() != nil` branch of ExitShowStatement ran before the
// FEATURES branch and swallowed the `for version` alternative.
func TestShowFeaturesForVersionRoutesToFeatures(t *testing.T) {
	for _, src := range []string{
		"show features for version 10.24;",
		"list features for version 10.24;",
		"mdl 1;\nlist features for version 10.24;",
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("%q: parse errors: %v", src, errs)
		}
		got := prog.Statements[len(prog.Statements)-1]
		fs, ok := got.(*ast.ShowFeaturesStmt)
		if !ok {
			t.Fatalf("%q: got %T, want *ast.ShowFeaturesStmt", src, got)
		}
		if fs.ForVersion != "10.24" {
			t.Errorf("%q: ForVersion = %q, want 10.24", src, fs.ForVersion)
		}
	}
	// Control: the bare `show version` still routes to the session version.
	prog, _ := Build("show version;")
	if s, ok := prog.Statements[0].(*ast.ShowStmt); !ok || s.ObjectType != ast.ShowVersion {
		t.Errorf("show version: got %#v, want ShowStmt{ShowVersion}", prog.Statements[0])
	}
}
