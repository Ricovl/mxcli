// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkLocaleNumberInJSON flags formatDecimal without a locale argument inside a
// string concatenation that builds JSON — MDL-JSONNUM01, ako/mxcli#982.
//
// formatDecimal(x, '0.00') formats in the CURRENT USER's language, so the same
// microflow writes `12.50` for an English user and `12,50` for a Dutch one, and
// the JSON is invalid only for the second ("Data is not valid JSON" in a chart
// widget that works for its author). Measured on 11.13:
//
//	formatDecimal(1234.5, '0.00', 'nl-NL')  → 1234,50
//	formatDecimal(1234.5, '0.00', 'nl_NL')  → the user's default: the underscore
//	                                          tag is silently ignored
//	toString(round(0.0000001, 2))           → a plain decimal, no exponent
//
// It is a heuristic, so it is info: "builds JSON" means the same `+` chain has a
// string literal containing `{` or `":`, which is how a hand-built JSON object
// looks and an ordinary display string ("Total: " + …) does not. A third
// argument of any value is accepted; checking the tag's spelling is left to the
// suggestion.
func (v *microflowValidator) checkLocaleNumberInJSON(label string, expr ast.Expression) {
	if expr == nil {
		return
	}
	reported := false
	var walk func(e ast.Expression, inChain bool)
	walk = func(e ast.Expression, inChain bool) {
		switch n := e.(type) {
		case *ast.SourceExpr:
			walk(n.Expression, inChain)
		case *ast.ParenExpr:
			walk(n.Inner, false)
		case *ast.BinaryExpr:
			if n.Operator == "+" && !inChain && !reported {
				var parts []ast.Expression
				flattenConcat(n, &parts)
				if call := localelessFormatDecimalInJSON(parts); call != nil {
					reported = true
					v.addViolation("MDL-JSONNUM01", linter.SeverityInfo,
						fmt.Sprintf("%s builds JSON with formatDecimal(…) and no locale: it formats in the "+
							"user's language, so a Dutch user gets '12,50' and the JSON is invalid for them only", label),
						"Use toString(round(x, 2)) — locale-independent, no exponent — or pass a hyphenated "+
							"locale: formatDecimal(x, '0.00', 'en-US'). An underscore tag ('en_US') is silently ignored.")
				}
			}
			isPlus := n.Operator == "+"
			walk(n.Left, isPlus)
			walk(n.Right, isPlus)
		case *ast.UnaryExpr:
			walk(n.Operand, false)
		case *ast.FunctionCallExpr:
			for _, a := range n.Arguments {
				walk(a, false)
			}
		case *ast.IfThenElseExpr:
			walk(n.Condition, false)
			walk(n.ThenExpr, false)
			walk(n.ElseExpr, false)
		}
	}
	walk(expr, false)
}

// flattenConcat collects the operands of a `+` chain, looking through the
// SourceExpr wrapper but not through parentheses (a parenthesised `+` may be
// arithmetic).
func flattenConcat(e ast.Expression, out *[]ast.Expression) {
	switch n := e.(type) {
	case *ast.SourceExpr:
		flattenConcat(n.Expression, out)
	case *ast.BinaryExpr:
		if n.Operator == "+" {
			flattenConcat(n.Left, out)
			flattenConcat(n.Right, out)
			return
		}
		*out = append(*out, e)
	default:
		*out = append(*out, e)
	}
}

func localelessFormatDecimalInJSON(parts []ast.Expression) *ast.FunctionCallExpr {
	looksJSON := false
	var call *ast.FunctionCallExpr
	for _, p := range parts {
		for {
			if s, ok := p.(*ast.SourceExpr); ok {
				p = s.Expression
				continue
			}
			break
		}
		switch n := p.(type) {
		case *ast.LiteralExpr:
			if s, ok := n.Value.(string); ok && n.Kind == ast.LiteralString &&
				(strings.Contains(s, "{") || strings.Contains(s, `":`)) {
				looksJSON = true
			}
		case *ast.FunctionCallExpr:
			if strings.EqualFold(n.Name, "formatDecimal") && len(n.Arguments) < 3 && call == nil {
				call = n
			}
		}
	}
	if looksJSON {
		return call
	}
	return nil
}
