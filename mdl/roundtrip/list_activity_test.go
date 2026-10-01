// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
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

	// describe writes the statements in every language (ako/mxcli#840): the
	// default, outside any script, and asked for mdl 0, where the call form it
	// wrote before the freeze is a deprecated spelling.
	plain, err := h.describe(listActivityTarget)
	if err != nil {
		t.Fatal(err)
	}
	mdl0, err := h.describeAs(langver.V0, listActivityTarget)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"the default describe": plain, "describe --mdl 0": mdl0} {
		if !strings.Contains(out, "= head $Feedbacks;") || strings.Contains(out, "= head($Feedbacks);") {
			t.Errorf("%s does not write the statement form:\n%s", name, out)
		}
	}
}

// describeAs runs describe asked for language v, as `mxcli describe --mdl <v>`
// does, and restores the default (mdl 1) after.
func (h *harness) describeAs(v langver.Version, target string) (string, error) {
	h.t.Helper()
	h.exe.SetDescribeLanguage(v)
	defer h.exe.SetDescribeLanguage(langver.Frozen)
	return h.describe(target)
}

// describeUnder runs describe inside a script with the given header, in that
// header's language: "" (a headerless script) describes in mdl 0, as
// `describe --mdl 0` does, so an mdl 0 leg stays one now that a plain
// describe writes mdl 1 (ako/mxcli#714).
func (h *harness) describeUnder(header, target string) string {
	h.t.Helper()
	h.exe.SetDescribeLanguage(langver.ScanHeader(header))
	defer h.exe.SetDescribeLanguage(langver.Frozen)
	if err := h.exec(header + "\ndescribe " + target + ";"); err != nil {
		h.t.Fatalf("describe %s under %q: %v", target, header, err)
	}
	return h.out.String()
}

// withoutHeader drops the `mdl 1;` line a description starts with since the
// freeze (ako/mxcli#714), for a control that reads the description's text in
// the other language.
func withoutHeader(s string) string {
	if first, rest, ok := strings.Cut(s, "\n"); ok && langver.IsHeaderLine(first) {
		return rest
	}
	return s
}
