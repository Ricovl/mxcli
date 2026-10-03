// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#950 item 1: describe prints a stored delete-with-close-page as
// `delete close page`, and exec of that wrote ClosePage=false — the visitor set
// the flag, the builder's delete case dropped it. A describe → exec round trip
// silently changed what the button does. Every page action that stores a
// ClosePage flag must carry it describe → parse → build, in both directions
// (the false half is the control: a builder that always wrote true would pass
// the true half).
func TestPageActionClosePage_DescribeParsesBackToTheSameFlag(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	for _, typ := range []string{"Forms$DeleteClientAction", "Forms$SaveChangesClientAction", "Forms$CancelChangesClientAction"} {
		for _, closePage := range []bool{true, false} {
			stored := map[string]any{"$Type": typ, "ClosePage": closePage, "DisabledDuringExecution": true}
			mdl := renderClientActionMDL(ctx, stored)
			prog, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
				"  actionbutton b1 (Caption: 'Go', Action: " + mdl + ")\n}")
			if len(errs) > 0 {
				t.Fatalf("%s: describe output %q does not parse: %v", typ, mdl, errs)
			}
			a := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Properties["Action"].(*ast.ActionV3)
			built, err := (&pageBuilder{}).buildClientActionV3(a)
			if err != nil {
				t.Fatalf("%s: build %q: %v", typ, mdl, err)
			}
			var got bool
			switch x := built.(type) {
			case *pages.DeleteClientAction:
				got = x.ClosePage
			case *pages.SaveChangesClientAction:
				got = x.ClosePage
			case *pages.CancelChangesClientAction:
				got = x.ClosePage
			default:
				t.Fatalf("%s: %q built %T", typ, mdl, built)
			}
			if got != closePage {
				t.Errorf("%s ClosePage=%v: describe printed %q, which builds ClosePage=%v", typ, closePage, mdl, got)
			}
		}
	}
}
