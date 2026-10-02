// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"bytes"
	"strings"
	"testing"
)

// `check` formats its tiers one batch at a time; each batch printed its own
// "N issues" line, so a warning in one tier and an error in another read as two
// partial counts. NoSummary leaves the line out and WriteSummary prints the one
// total over every batch.
func TestTextFormatterNoSummaryAndOneTotal(t *testing.T) {
	warn := []Violation{{RuleID: "W1", Severity: SeverityWarning, Message: "w"}}
	errs := []Violation{{RuleID: "E1", Severity: SeverityError, Message: "e"}}

	var out bytes.Buffer
	f := &TextFormatter{NoSummary: true}
	f.Format(warn, &out)
	f.Format(errs, &out)
	if strings.Contains(out.String(), "issues:") {
		t.Fatalf("a batch printed its own summary:\n%s", out.String())
	}
	WriteSummary(&out, append(warn, errs...), 0)
	if got := out.String(); !strings.Contains(got, "2 issues: 1 errors, 1 warnings, 0 info") {
		t.Errorf("want one total over both batches, got:\n%s", got)
	}

	// Control: without NoSummary each batch still prints its line.
	out.Reset()
	(&TextFormatter{}).Format(warn, &out)
	if !strings.Contains(out.String(), "1 issues: 0 errors, 1 warnings, 0 info") {
		t.Errorf("the default formatter must keep its summary:\n%s", out.String())
	}
}
