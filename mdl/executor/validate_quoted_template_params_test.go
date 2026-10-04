// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func quotedParamWidget(t *testing.T, param string) *ast.WidgetV3 {
	t.Helper()
	prog := parseMDL(t, `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dynamictext d (Content: 'x {1}', ContentParams: ({1} = `+param+`))
};`)
	s := prog.Statements[0].(*ast.CreatePageStmtV3)
	return allPageWidgets(s)[0]
}

// ako/mxcli#969 item 5: a quoted parameter that reads like an expression is
// stored as its text; say so. The negatives are the false hints a looser
// heuristic would give on ordinary text.
func TestQuotedTemplateParam_Hint(t *testing.T) {
	for param, want := range map[string]bool{
		`'formatDateTime($Log/Date, ''d MMM'')'`: true,
		`'toString($Order/Total)'`:               true,
		`'$currentObject/Name'`:                  true,
		`'$Order'`:                               true,
		// controls — text, not expressions
		`'Total (incl. VAT)'`:    false,
		`'length (cm)'`:          false,
		`'not(yet)'`:             false,
		`'Price in $'`:           false,
		`'Cost: $5'`:             false,
		`'Sum(of parts)'`:        false,
		`'a' + 'b'`:              false,
		`toString($Order/Total)`: false,
	} {
		got := validateQuotedTemplateParams(quotedParamWidget(t, param), "page M.P")
		if (len(got) == 1) != want || len(got) > 1 {
			t.Errorf("%s: got %v, want hint=%v", param, got, want)
			continue
		}
		if want && (got[0].RuleID != "MDL-PARAMQUOTE01" || !strings.Contains(got[0].Message, "drop the quotes")) {
			t.Errorf("%s: unexpected violation %+v", param, got[0])
		}
	}
}
