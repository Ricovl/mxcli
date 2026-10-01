// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Option C for the #823 change (ako/mxcli#714): a text-template parameter
// bound to a non-String attribute of a parameter or data view — `{1} =
// $Task.Status` — is stored as Studio Pro stores it, an attribute reference,
// under every language version. Before #823 mxcli wrapped it in
// `toString($Task/Status)`, so an existing mdl 0 script now renders the value
// through the attribute's formatting (an enumeration's caption, a Decimal's
// precision) instead of toString's. Under mdl 0 that is reported as
// MDL-V1-TEMPLATEATTR, naming the explicit expression that keeps the old
// output.

func templateWarningPB(v langver.Version) (*pageBuilder, *bytes.Buffer) {
	pb := templateBindingPB(false)
	var out bytes.Buffer
	pb.ctx = &ExecContext{Output: &out, LanguageVersion: v}
	return pb, &out
}

func TestTemplateAttrWarning_Mdl0NonStringAttribute(t *testing.T) {
	for _, value := range []string{"$Task.Status", "$Task.IsLocal"} {
		pb, out := templateWarningPB(langver.V0)
		p := &pages.ClientTemplateParameter{}
		pb.resolveTemplateAttributePathFull(value, p)
		if p.Expression != "" || p.AttributeRef == "" {
			t.Errorf("%s: stored Expression %q / AttributeRef %q, want the attribute reference (option C keeps it)",
				value, p.Expression, p.AttributeRef)
		}
		attr := strings.TrimPrefix(value, "$Task.")
		if !strings.Contains(out.String(), "MDL-V1-TEMPLATEATTR") ||
			!strings.Contains(out.String(), "toString($Task/"+attr+")") {
			t.Errorf("%s under mdl 0: want an MDL-V1-TEMPLATEATTR warning naming toString($Task/%s), got %q", value, attr, out.String())
		}
		// Every coded warning points at its registry entry (ako/mxcli#714).
		if !strings.HasSuffix(strings.TrimSpace(out.String()), "(mxcli help MDL-V1-TEMPLATEATTR)") {
			t.Errorf("%s under mdl 0: the warning must end with (mxcli help MDL-V1-TEMPLATEATTR), got %q", value, out.String())
		}
	}
}

// Controls: a String attribute was never wrapped, and under mdl 1 the binding
// is simply the language's meaning — neither warns.
func TestTemplateAttrWarning_Controls(t *testing.T) {
	for _, tc := range []struct {
		name  string
		v     langver.Version
		value string
	}{
		{"String attribute, mdl 0", langver.V0, "$Task.Title"},
		{"enumeration attribute, mdl 1", langver.V1, "$Task.Status"},
	} {
		pb, out := templateWarningPB(tc.v)
		p := &pages.ClientTemplateParameter{}
		pb.resolveTemplateAttributePathFull(tc.value, p)
		if p.AttributeRef == "" {
			t.Errorf("%s: no AttributeRef", tc.name)
		}
		if strings.Contains(out.String(), "MDL-V1-TEMPLATEATTR") {
			t.Errorf("%s: warned %q", tc.name, out.String())
		}
	}
}

// The escape hatch the warning names: `{1} = toString($Task/Status)` is an
// expression, stored as the parameter's Expression — not resolved as an
// attribute path, which made it a bogus attribute name — and not warned.
func TestTemplateParam_ExplicitExpressionIsStoredAsExpression(t *testing.T) {
	for _, expr := range []string{
		"toString($Task/Status)",
		"formatDecimal($Task/Amount, '#,##0.00')",
		"$Task/Title + ' (draft)'",
	} {
		pb, out := templateWarningPB(langver.V0)
		ps := pb.buildClientTemplateParams([]ast.ParamAssignmentV3{{Index: 1, Value: expr}})
		if len(ps) != 1 {
			t.Fatalf("%s: %d params", expr, len(ps))
		}
		if ps[0].Expression != expr || ps[0].AttributeRef != "" || ps[0].SourceVariable != "" {
			t.Errorf("%s: stored Expression %q, AttributeRef %q, SourceVariable %q; want the expression as written",
				expr, ps[0].Expression, ps[0].AttributeRef, ps[0].SourceVariable)
		}
		if out.Len() != 0 {
			t.Errorf("%s: warned %q", expr, out.String())
		}
	}
	// Control: an attribute reference through the same path still binds.
	pb, _ := templateWarningPB(langver.V0)
	ps := pb.buildClientTemplateParams([]ast.ParamAssignmentV3{{Index: 1, Value: "$Task.Title"}})
	if ps[0].Expression != "" || ps[0].AttributeRef != "M.Job.Title" {
		t.Errorf("$Task.Title: Expression %q, AttributeRef %q", ps[0].Expression, ps[0].AttributeRef)
	}
}

// describe prints a stored template-parameter expression as written, so
// `{1} = toString($Task/Status)` survives describe -> exec. It used to be
// unwrapped to `$Task.Status`, which now binds the attribute: a round trip
// that changed the page's output.
func TestDescribeTemplateParam_ToStringExpressionIsKept(t *testing.T) {
	ctx, _ := newMockCtx(t)
	for _, expr := range []string{"toString($Task/Status)", "toString($currentObject/CountAll)"} {
		raw := map[string]any{
			"$Type": "Forms$DynamicText",
			"Name":  "num",
			"Content": map[string]any{
				"$Type": "Forms$ClientTemplate",
				"Template": map[string]any{
					"$Type": "Texts$Text",
					"Items": []any{
						map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "{1}"},
					},
				},
				"Parameters": []any{
					map[string]any{"$Type": "Forms$ClientTemplateParameter", "Expression": expr},
				},
			},
		}
		got := parseRawWidget(ctx, raw)
		if len(got) != 1 || len(got[0].Parameters) != 1 || got[0].Parameters[0] != expr {
			t.Errorf("describe printed %v, want [%s]", got[0].Parameters, expr)
		}
	}
}
