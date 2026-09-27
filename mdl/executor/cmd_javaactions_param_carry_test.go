// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// A Java action parameter's Description and Category have no MDL spelling:
// DESCRIBE prints the description as a `--` comment, which the parser drops. So
// once describe printed `create or modify java action` (ako/mxcli#705 item 5),
// re-executing its unchanged output deleted every parameter description —
// measured on PedApp's FeedbackModule.ValidateEmail ("Email address to
// validate"). A rewrite carries them from the stored parameter of the same name.
func TestJavaActionRewriteCarriesParameterDescriptionAndCategory(t *testing.T) {
	mod := mkModule("MyModule")
	stored := storedExposedAction(mod)
	stored.Parameters = []*javaactions.JavaActionParameter{
		{Name: "EmailAddress", Description: "Email address to validate", Category: "Input", IsRequired: true},
		{Name: "Dropped", Description: "not in the statement"},
	}
	ctx, captured := exposeRewriteCtx(t, stored, mod)

	s := rewriteStmt(false)
	s.Parameters = []ast.JavaActionParam{
		{Name: "EmailAddress", Type: ast.DataType{Kind: ast.TypeString}, IsRequired: true},
		// Control: a parameter the stored action does not have gets nothing.
		{Name: "NewOne", Type: ast.DataType{Kind: ast.TypeString}},
	}
	assertNoError(t, execCreateJavaAction(ctx, s))

	got := *captured
	if got == nil {
		t.Fatal("UpdateJavaAction was never called")
	}
	if len(got.Parameters) != 2 {
		t.Fatalf("got %d parameters, want 2", len(got.Parameters))
	}
	if p := got.Parameters[0]; p.Description != "Email address to validate" || p.Category != "Input" {
		t.Errorf("EmailAddress: description %q, category %q — want the stored ones carried", p.Description, p.Category)
	}
	if p := got.Parameters[1]; p.Description != "" || p.Category != "" {
		t.Errorf("NewOne: description %q, category %q — a new parameter has none to carry", p.Description, p.Category)
	}
}
