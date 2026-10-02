// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// preflightScript draws one INFO note (MDL067, a bare commit) and one WARNING
// (MDL089, a doc comment on a statement that cannot store it).
const preflightScript = `create microflow M.F ($X: M.E)
begin
  commit $X;
end;

/** lost */
drop microflow if exists M.Old;
`

func runPreflight(t *testing.T, src string, showInfo bool) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out bytes.Buffer
	refusal := execPreflight(executor.New(io.Discard), prog, "", "s.mdl", false, showInfo,
		deprecation.Warn, &out, false)
	if refusal != "" {
		t.Fatalf("unexpected refusal: %s", refusal)
	}
	return out.String()
}

// TestExecPreflightSummarizesInfoNotes: exec prints warnings and errors in
// full and info notes as one count line pointing at `check`. On report pages
// MDL-WIDGET15 alone was ~35 notes a run, burying the warnings that matter.
func TestExecPreflightSummarizesInfoNotes(t *testing.T) {
	out := runPreflight(t, preflightScript, false)
	if !strings.Contains(out, "[MDL089]") {
		t.Errorf("the warning must print in full, got:\n%s", out)
	}
	if strings.Contains(out, "[MDL067]") {
		t.Errorf("the info note must not print in full, got:\n%s", out)
	}
	if !strings.Contains(out, "1 info note not shown") || !strings.Contains(out, "mxcli check s.mdl") {
		t.Errorf("want a count line naming `mxcli check s.mdl`, got:\n%s", out)
	}
	// The report's own summary must not claim "0 info" for a script that has one.
	if !strings.Contains(out, "2 issues: 0 errors, 1 warnings, 1 info (not shown)") {
		t.Errorf("want the summary to count the omitted note, got:\n%s", out)
	}
}

// TestExecPreflightVerboseShowsInfoNotes is the control: --verbose prints the
// same note in full, so its absence above is the summary's doing.
func TestExecPreflightVerboseShowsInfoNotes(t *testing.T) {
	out := runPreflight(t, preflightScript, true)
	for _, want := range []string{"[MDL089]", "[MDL067]"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %s in full with --verbose, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "not shown") {
		t.Errorf("nothing is hidden with --verbose, got:\n%s", out)
	}
}

// TestExecPreflightInfoOnly: a script with nothing but info notes prints the
// count line alone — not an empty report or "No issues found."
func TestExecPreflightInfoOnly(t *testing.T) {
	out := runPreflight(t, "create microflow M.F ($X: M.E)\nbegin\n  commit $X;\nend;\n", false)
	if strings.TrimSpace(out) == "" || strings.Contains(out, "No issues found") || strings.Contains(out, "[MDL067]") {
		t.Errorf("want only the count line, got:\n%s", out)
	}
	if !strings.Contains(out, "1 info note not shown") {
		t.Errorf("want the count line, got:\n%s", out)
	}
}
