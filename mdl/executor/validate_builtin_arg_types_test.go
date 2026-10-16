// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// TestValidateMicroflow_BuiltinArgumentTypes guards mendixlabs/mxcli#1515 on
// the tier that runs without a project. `formatDecimal($d, 2)` hands a number
// to the String pattern and mxbuild refuses it (CE0117 on 11.6.6 and 11.15.0),
// yet `mxcli check` printed "Check passed!": the built-in signature check
// compared only argument COUNTS, and only on the `-p --references` tier. The
// kinds it needs here — literals, parameters, declared variables — are all in
// the script.
func TestValidateMicroflow_BuiltinArgumentTypes(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantRule string // "" = no E005/E006
		wantMsg  string
	}{
		{
			name:     "issue repro: number as the formatDecimal pattern",
			body:     "declare $r String = formatDecimal($d, 2);\n  return $r;",
			wantRule: "E005",
			wantMsg:  "formatDecimal() argument 2",
		},
		{
			name:     "control: String pattern",
			body:     "declare $r String = formatDecimal($d, '#,##0.00');\n  return $r;",
			wantRule: "",
		},
		{
			name:     "arity without a project",
			body:     "declare $r String = formatDecimal($d, '#', 'en', 4);\n  return $r;",
			wantRule: "E006",
			wantMsg:  "formatDecimal()",
		},
		{
			name:     "a declared variable's kind is used",
			body:     "declare $n Integer = 3;\n  declare $r String = toUpperCase($n);\n  return $r;",
			wantRule: "E005",
			wantMsg:  "toUpperCase() argument 1",
		},
		{
			name:     "inside a create member value",
			body:     "$o = create Tmp.Item (Lbl = formatDecimal($d, 2));\n  return 'x';",
			wantRule: "E005",
		},
		{
			name:     "unknown variable stays quiet",
			body:     "declare $r String = formatDecimal($nope, '#');\n  return $r;",
			wantRule: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mf := buildMF(t, "create microflow Tmp.F1 ($d: Decimal) returns String\nbegin\n  "+c.body+"\nend;")
			var got []string
			for _, v := range ValidateMicroflow(mf) {
				if v.RuleID != "E005" && v.RuleID != "E006" {
					continue
				}
				got = append(got, v.RuleID+": "+v.Message)
				if v.Severity != linter.SeverityError {
					t.Errorf("%s must be an error, got %v", v.RuleID, v.Severity)
				}
			}
			if c.wantRule == "" {
				if len(got) != 0 {
					t.Fatalf("want no built-in signature violation, got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.HasPrefix(got[0], c.wantRule+": ") || !strings.Contains(got[0], c.wantMsg) {
				t.Fatalf("want one %s mentioning %q, got %v", c.wantRule, c.wantMsg, got)
			}
		})
	}
}

// TestValidateNanoflow_BuiltinArgumentTypes: the nanoflow walk shares the
// expression sites, and its kinds come from seedPrimitiveKinds.
func TestValidateNanoflow_BuiltinArgumentTypes(t *testing.T) {
	nf := buildNF(t, `create nanoflow Tmp.NF ($d: Decimal) returns String
begin
  return formatDecimal($d, 2);
end;`)
	found := false
	for _, v := range ValidateNanoflow(nf) {
		if v.RuleID == "E005" {
			found = true
		}
	}
	if !found {
		t.Fatal("want E005 for formatDecimal($d, 2) in a nanoflow")
	}
}
