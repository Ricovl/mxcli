// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// jsonNumViolations parses real MDL and returns the MDL-JSONNUM01 hits, going
// through the visitor so the expression shape is the one users actually get.
func jsonNumViolations(t *testing.T, body string) int {
	t.Helper()
	prog := parseMDL(t, `create or replace microflow Shop.Build($Total: Decimal) returns String
begin
  declare $Json String = '';
  `+body+`
  return $Json;
end;`)
	n := 0
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, v := range ValidateMicroflow(mf) {
			if v.RuleID == "MDL-JSONNUM01" {
				n++
			}
		}
	}
	return n
}

// The skill's old idiom (ako/mxcli#982): JSON built by concatenation with a
// locale-less formatDecimal, which writes '12,50' for a Dutch user.
func TestJSONNum_LocalelessFormatDecimalInJSON(t *testing.T) {
	for name, body := range map[string]string{
		"object key": `set $Json = $Json + ',"v":' + formatDecimal($Total, '0.00') + '}';`,
		"brace only": `set $Json = '{' + formatDecimal($Total, '0.00') + '}';`,
		"in declare": `declare $J String = '{"v":' + formatDecimal($Total, '0.00') + '}';`,
	} {
		if got := jsonNumViolations(t, body); got != 1 {
			t.Errorf("%s: %d MDL-JSONNUM01, want 1", name, got)
		}
	}
}

// Controls: the fixes, and the same call outside JSON, are not flagged.
func TestJSONNum_NotFlagged(t *testing.T) {
	for name, body := range map[string]string{
		"toString(round)":  `set $Json = $Json + ',"v":' + toString(round($Total, 2)) + '}';`,
		"explicit locale":  `set $Json = $Json + ',"v":' + formatDecimal($Total, '0.00', 'en-US') + '}';`,
		"display string":   `set $Json = 'Total: ' + formatDecimal($Total, '0.00');`,
		"no concatenation": `set $Json = formatDecimal($Total, '0.00');`,
	} {
		if got := jsonNumViolations(t, body); got != 0 {
			t.Errorf("%s: %d MDL-JSONNUM01, want 0", name, got)
		}
	}
}
