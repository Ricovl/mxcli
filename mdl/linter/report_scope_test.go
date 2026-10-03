// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ako/mxcli#953 item 6: `report --modules A,B` scores the project's own
// modules. The rules already skip unselected modules through the LintContext
// filter, but some findings are not about a module at all (project security,
// role mappings) or are reported in a module other than the one iterated; the
// score is computed over the findings located in a selected module only.
func TestScopeToModules(t *testing.T) {
	vs := []Violation{
		{RuleID: "SEC001", Severity: SeverityWarning, Location: Location{Module: "MyFirstModule"}},
		{RuleID: "SEC001", Severity: SeverityWarning, Location: Location{Module: "Administration"}},
		{RuleID: "CONV008", Severity: SeverityInfo, Location: Location{Module: ""}},
		{RuleID: "MPR002", Severity: SeverityWarning, Location: Location{Module: "Other"}},
	}
	got := ScopeToModules(vs, []string{"MyFirstModule", "Other"})
	if len(got) != 2 || got[0].Location.Module != "MyFirstModule" || got[1].Location.Module != "Other" {
		t.Fatalf("want the MyFirstModule and Other findings only, got %+v", got)
	}
	// Control: no selection is no scoping.
	if all := ScopeToModules(vs, nil); len(all) != len(vs) {
		t.Fatalf("no module selection must keep every finding, got %d of %d", len(all), len(vs))
	}

	// The score moves with the scope: the unscoped report counts the
	// Administration finding against Security, the scoped one does not.
	full := BuildReport("App", "d", vs)
	scoped := BuildReport("App", "d", got)
	if scoped.OverallScore <= full.OverallScore {
		t.Fatalf("scoped score %v should exceed unscoped %v", scoped.OverallScore, full.OverallScore)
	}

	// The scope is stated in every format, so a module score is never read as
	// the project's.
	scoped.Modules = []string{"MyFirstModule", "Other"}
	for _, format := range []string{"markdown", "html", "json"} {
		var buf bytes.Buffer
		if err := GetReportFormatter(format).FormatReport(scoped, &buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "MyFirstModule") || !strings.Contains(buf.String(), "Other") {
			t.Errorf("%s output does not name the selected modules:\n%s", format, buf.String())
		}
		if format == "json" {
			var jr struct {
				Modules []string `json:"modules"`
			}
			if err := json.Unmarshal(buf.Bytes(), &jr); err != nil || len(jr.Modules) != 2 {
				t.Errorf("json modules = %v (err %v)", jr.Modules, err)
			}
		}
	}
}
