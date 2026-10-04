// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateQuotedTemplateParams emits an info hint (MDL-PARAMQUOTE01) for a
// template parameter whose value is a quoted string that reads like an
// expression, such as a quoted formatDateTime($Log/Date, …) call.
//
// The quotes make it a String literal, which is exactly what is stored — the
// page then shows the expression's TEXT. That is valid, builds clean and is
// occasionally intended, so it is a hint and not an error; but nothing said so,
// and the author saw the source text on the rendered page (ako/mxcli#969).
//
// The heuristic is deliberately narrow, so plain text with parentheses or a
// dollar sign is left alone:
//   - the whole literal is a call of a known Mendix built-in function, with
//     the parenthesis directly after the name (`toString(...)`,
//     `formatDateTime(...)`; prose writes "length (cm)" with a space), or
//   - it contains a variable path, `$Name/Attr`, or is exactly `$Name`.
//
// "Total (incl. VAT)", "Price in $" and "Cost: $5" match neither.
func validateQuotedTemplateParams(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	var out []linter.Violation
	for _, set := range []struct {
		prop   string
		params []ast.ParamAssignmentV3
	}{
		{"ContentParams", w.GetContentParams()},
		{"CaptionParams", w.GetCaptionParams()},
	} {
		for _, p := range set.params {
			s, ok := p.Value.(string)
			if !ok {
				continue
			}
			inner, quoted := singleStringLiteral(s)
			if !quoted || !looksLikeExpression(inner) {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-PARAMQUOTE01",
				Severity: linter.SeverityInfo,
				Message: fmt.Sprintf(
					"%s: %s {%d} = %s is quoted, so it is stored as literal text and the page shows it verbatim — "+
						"this quoted parameter looks like an expression; drop the quotes to make it one",
					locationPrefix+" "+widgetLabel(w.Name, w.Type), set.prop, p.Index, s),
				Suggestion: fmt.Sprintf("{%d} = %s", p.Index, strings.ReplaceAll(inner, "''", "'")),
			})
		}
	}
	return out
}

// singleStringLiteral reports whether s is exactly one Mendix string literal
// (quoted, with a doubled quote as the escape) and returns its raw body.
func singleStringLiteral(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	body := s[1 : len(s)-1]
	// A lone quote inside would end the literal early: 'a' + 'b' is not one.
	if strings.Count(strings.ReplaceAll(body, "''", ""), "'") != 0 {
		return "", false
	}
	return body, true
}

var (
	quotedCallRe = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9]*)\(.*\)\s*$`)
	quotedPathRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*/[A-Za-z_]`)
	quotedVarRe  = regexp.MustCompile(`^\s*\$[A-Za-z_][A-Za-z0-9_]*\s*$`)
)

func looksLikeExpression(body string) bool {
	if m := quotedCallRe.FindStringSubmatch(body); m != nil {
		// `not` is a keyword-shaped function that prose uses too ("not(yet)").
		if _, ok := exprcheck.FuncReturnKind(m[1]); ok && m[1] != "not" {
			return true
		}
	}
	return quotedPathRe.MatchString(body) || quotedVarRe.MatchString(body)
}
