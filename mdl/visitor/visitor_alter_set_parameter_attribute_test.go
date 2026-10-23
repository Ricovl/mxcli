// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1622: `set (eventDataAttribute: $Ctx.EventData) on chart1` was a
// parse error — "takes a plain value, not an expression" — so an attribute of
// a page parameter could not be bound by ALTER at all. `$Param.Attr` is the
// spelling CREATE takes for that binding; it now reaches the executor, which
// knows the property's kind and decides. Any longer expression is refused here
// as before.
func TestAlterSet_ParameterAttributeReachesTheExecutor(t *testing.T) {
	for src, want := range map[string]string{
		`$Ctx.EventData`: "$Ctx.EventData",
		`$Ctx."Type"`:    "$Ctx.Type", // identifier quotes stripped, as in CREATE
	} {
		prog, errs := Build(`alter page M.P { set (eventDataAttribute: ` + src + `) on chart1; };`)
		if len(errs) > 0 {
			t.Errorf("%s: %v", src, errs)
			continue
		}
		op := prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp)
		got, ok := op.Properties["eventDataAttribute"].(*ast.WidgetExpressionV3)
		if !ok || got.Text != want {
			t.Errorf("%s: value = %#v, want *ast.WidgetExpressionV3{Text: %q}", src, op.Properties["eventDataAttribute"], want)
		}
	}

	// Controls: an expression is still refused at parse.
	for _, src := range []string{`$Ctx.EventData + 'x'`, `$Ctx/EventData`} {
		_, errs := Build(`alter page M.P { set (eventDataAttribute: ` + src + `) on chart1; };`)
		if len(errs) == 0 || !strings.Contains(errs[0].Error(), "takes a plain value, not an expression") {
			t.Errorf("%s: want the plain-value refusal, got %v", src, errs)
		}
	}
}
