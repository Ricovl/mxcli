// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"regexp"
	"strings"
	"testing"
)

// listActivityRows is one statement per row of the §4 Microflows table of
// PROPOSAL_mdl_beta_syntax_freeze.md, written against PedApp's
// FeedbackModule.Feedback. Each is also the exact line describe must print
// back under mdl 1 (#733).
var listActivityRows = []string{
	"$Open = filter $Feedbacks by Subject = 'Open';",
	"$Wide = filter $Feedbacks where $currentObject/ScreenWidth > 1000;",
	"$Match = find $Feedbacks by ScreenWidth = 3;",
	"$Tall = find $Feedbacks where $currentObject/ScreenHeight > 2;",
	"$Sorted = sort $Feedbacks by Subject desc, ScreenWidth asc;",
	"$First = head $Feedbacks;",
	"$Rest = tail $Feedbacks;",
	"$Page = range $Feedbacks offset 20 limit 10;",
	"$All = union $Feedbacks with $Others;",
	"$Both = intersect $Feedbacks with $Others;",
	"$Left = subtract $Others from $Feedbacks;",
	"$Has = contains $One in $Feedbacks;",
	"$Same = equals $Feedbacks and $Others;",
	"$N = count $Feedbacks;",
	"$Total = sum $Feedbacks by ScreenWidth;",
	"$Double = sum $Feedbacks of $currentObject/ScreenWidth * 2;",
	"$Avg = average $Feedbacks by ScreenWidth;",
	"$Min = minimum $Feedbacks by ScreenHeight;",
	"$Max = maximum $Feedbacks by ScreenHeight;",
	"$AllShown = all $Feedbacks where $currentObject/_showEmail;",
	"$AnyShown = any $Feedbacks where $currentObject/_showEmail;",
	"$Csv = reduce $Feedbacks from '' as String using $currentResult + $currentObject/Subject;",
}

const listActivityTarget = "microflow MyFirstModule.ListActivities"

func listActivityScript() string {
	return "mdl 1;\ncreate microflow MyFirstModule.ListActivities (\n" +
		"  $Feedbacks: List of FeedbackModule.Feedback,\n" +
		"  $Others: List of FeedbackModule.Feedback,\n" +
		"  $One: FeedbackModule.Feedback\n)\nreturns Boolean\nbegin\n  " +
		strings.Join(listActivityRows, "\n  ") + "\n  return $Has;\nend;\n"
}

var positionLine = regexp.MustCompile(`(?m)^\s*@(position|anchor|curve|merge)\b.*\n`)

// Every row of the §4 table parses, executes on the Studio Pro-authored
// fixture, and reads back through describe under the header as the same
// statement; the described microflow executes back to the same description.
func TestPedAppListActivitiesUnderMdl1(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	if err := h.exec(listActivityScript()); err != nil {
		t.Fatalf("exec under mdl 1: %v\n%s", err, h.out.String())
	}
	first := h.describeUnder("mdl 1;", listActivityTarget)
	body := positionLine.ReplaceAllString(first, "")
	for _, row := range listActivityRows {
		if !strings.Contains(body, "  "+row+"\n") {
			t.Errorf("describe under mdl 1 does not print %q:\n%s", row, body)
		}
	}

	// PutGet: executing the mdl 1 description describes back to itself.
	if err := h.exec("mdl 1;\n" + first); err != nil {
		t.Fatalf("exec the mdl 1 description: %v\n%s", err, first)
	}
	if !strings.Contains(h.out.String(), "Unchanged microflow: MyFirstModule.ListActivities") {
		t.Errorf("GetPut: the mdl 1 description of an unchanged flow must write nothing, got:\n%s", h.out.String())
	}
	if again := h.describeUnder("mdl 1;", listActivityTarget); again != first {
		t.Errorf("describe -> exec -> describe changed the microflow:\n%s", lineDiff(first, again))
	}

	// Control: outside an mdl 1 script describe keeps the call form while mdl 1
	// is a preview, so the rows above are not what a plain describe prints.
	plain, err := h.describe(listActivityTarget)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "= head $Feedbacks;") || !strings.Contains(plain, "= head($Feedbacks);") {
		t.Errorf("a plain describe must keep the call form while mdl 1 is a preview:\n%s", plain)
	}
}

// describeUnder runs describe inside a script with the given header.
func (h *harness) describeUnder(header, target string) string {
	h.t.Helper()
	if err := h.exec(header + "\ndescribe " + target + ";"); err != nil {
		h.t.Fatalf("describe %s under %q: %v", target, header, err)
	}
	return h.out.String()
}
