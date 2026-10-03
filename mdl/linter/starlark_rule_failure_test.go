// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#952: a project's lint rules were written by a newer mxcli and read
// a struct field (document_noun) the binary on PATH did not expose. Three
// rules crashed, each crash was an error-severity finding, and the report
// scored the project 30 points lower for problems the project did not have.

// newerMxcliRule reads a field no struct has — what a rule written for a newer
// mxcli looks like to an older one.
const newerMxcliRule = `
RULE_ID = "NEWER001"
RULE_NAME = "Newer"
DESCRIPTION = "reads a field this mxcli does not expose"
CATEGORY = "Quality"
SEVERITY = "error"

def check():
    v = violation(message = "x")
    return [violation(message = v.document_noun_from_the_future)]
`

// brokenRule fails for a reason that is not a missing field.
const brokenRule = `
RULE_ID = "BROKEN001"
RULE_NAME = "Broken"
DESCRIPTION = "divides by zero"
CATEGORY = "Quality"
SEVERITY = "warning"

def check():
    return [violation(message = str(1 // 0))]
`

// findingRule is the control: a working rule whose finding must still count.
const findingRule = `
RULE_ID = "QUAL001"
RULE_NAME = "Finding"
DESCRIPTION = "always finds one thing"
CATEGORY = "Quality"
SEVERITY = "warning"

def check():
    return [violation(message = "a real finding")]
`

func loadRuleSource(t *testing.T, name, src string) *StarlarkRule {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, name, src)
	r, err := LoadStarlarkRule(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("LoadStarlarkRule(%s): %v", name, err)
	}
	return r
}

func TestStarlarkRule_MissingFieldIsNewerMxcliInfo(t *testing.T) {
	r := loadRuleSource(t, "newer.star", newerMxcliRule)
	vs := r.Check(&LintContext{})
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
	v := vs[0]
	if !v.RuleFailure {
		t.Error("not marked as a rule failure")
	}
	if v.Severity != SeverityInfo {
		t.Errorf("severity = %s, want info", v.Severity)
	}
	if !strings.HasPrefix(v.Message, "rule NEWER001 needs a newer mxcli (") ||
		!strings.Contains(v.Message, ".document_noun_from_the_future") {
		t.Errorf("message = %q", v.Message)
	}
	if strings.Contains(v.Message, "Traceback") {
		t.Errorf("message carries the backtrace: %q", v.Message)
	}
}

func TestStarlarkRule_OtherFailureStaysError(t *testing.T) {
	r := loadRuleSource(t, "broken.star", brokenRule)
	vs := r.Check(&LintContext{})
	if len(vs) != 1 || !vs[0].RuleFailure || vs[0].Severity != SeverityError {
		t.Fatalf("got %+v, want one error-severity rule failure", vs)
	}
	if !strings.HasPrefix(vs[0].Message, "Starlark rule error:") {
		t.Errorf("message = %q", vs[0].Message)
	}
}

// A severity configured for a rule is about its findings; it must not turn
// the rule's own failure back into an error (or hide a real crash).
func TestLinterRun_SeverityOverrideSkipsRuleFailures(t *testing.T) {
	l := New(&LintContext{})
	l.AddRule(loadRuleSource(t, "newer.star", newerMxcliRule))
	l.ConfigureRule("NEWER001", RuleConfig{Enabled: true, Severity: SeverityError})
	vs, err := l.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Severity != SeverityInfo {
		t.Errorf("got %+v, want the failure to stay info", vs)
	}
}

func TestBuildReport_RuleFailuresAreNotScored(t *testing.T) {
	l := New(&LintContext{})
	for name, src := range map[string]string{"newer.star": newerMxcliRule, "broken.star": brokenRule, "finding.star": findingRule} {
		l.AddRule(loadRuleSource(t, name, src))
	}
	all, err := l.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d violations, want 3: %+v", len(all), all)
	}

	report := BuildReport("Demo", "today", all)

	// The control finding still counts: one warning in Quality.
	if report.Summary.Total != 1 || report.Summary.Warnings != 1 || report.Summary.Errors != 0 {
		t.Errorf("summary = %+v, want only the control warning", report.Summary)
	}
	if len(report.Violations) != 1 || report.Violations[0].RuleID != "QUAL001" {
		t.Errorf("violations = %+v", report.Violations)
	}
	if len(report.RuleFailures) != 2 {
		t.Errorf("rule failures = %+v, want both failing rules", report.RuleFailures)
	}
	// The score is the control finding's alone.
	control := BuildReport("Demo", "today", []Violation{report.Violations[0]})
	if report.OverallScore != control.OverallScore {
		t.Errorf("score = %v, want %v (rule failures must not move it)", report.OverallScore, control.OverallScore)
	}
	if control.OverallScore == 100 {
		t.Error("control finding did not move the score; the comparison proves nothing")
	}

	// Every format lists the failures separately.
	var md bytes.Buffer
	if err := GetReportFormatter("markdown").FormatReport(report, &md); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "## Rules That Could Not Run") || !strings.Contains(md.String(), "needs a newer mxcli") {
		t.Errorf("markdown lacks the rule-failure section:\n%s", md.String())
	}
	var js bytes.Buffer
	if err := GetReportFormatter("json").FormatReport(report, &js); err != nil {
		t.Fatal(err)
	}
	var jr JSONReport
	if err := json.Unmarshal(js.Bytes(), &jr); err != nil {
		t.Fatal(err)
	}
	if len(jr.RuleFailures) != 2 || len(jr.Violations) != 1 {
		t.Errorf("json: %d rule failures, %d violations; want 2 and 1", len(jr.RuleFailures), len(jr.Violations))
	}
	var h bytes.Buffer
	if err := GetReportFormatter("html").FormatReport(report, &h); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.String(), "Rules That Could Not Run") {
		t.Error("html lacks the rule-failure section")
	}
}

// A builtin a newer mxcli added fails at load, as an undefined name; the
// skipped-file reason says what that usually means.
func TestLoadStarlarkRulesFromDir_UndefinedBuiltinNamesNewerMxcli(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "future.star", "def check():\n    return future_builtin()\n")
	_, failures, err := LoadStarlarkRulesFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || !strings.Contains(failures[0].Reason, "may need a newer mxcli") {
		t.Errorf("failures = %+v", failures)
	}
}
