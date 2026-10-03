// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// changeListTypeRe reads the Type of every Microflows$ChangeListAction in a
// unit's BSON: the action's $Type string, then its Type string element.
var changeListTypeRe = regexp.MustCompile(`(?s)Microflows\$ChangeListAction\x00.*?\x02Type\x00.{4}(\w+)\x00`)

// The four operations of a Change list activity, in a microflow and in a
// nanoflow. Clear (`clear $L;`) and Replace (stored "Set", `set $L = $M;`)
// are what describe printed before either executed back to the same action:
// `clear` did not parse, and `set` on a list wrote a Change variable action,
// which mxbuild refuses (CE7247). ako/mxcli#944, item 1.
func TestChangeListOperationsRoundTrip(t *testing.T) {
	rows := []string{
		"add $One to $Feedbacks;",
		"remove $One from $Feedbacks;",
		"clear $Feedbacks;",
		"set $Feedbacks = $Others;",
	}
	for _, kind := range []string{"microflow", "nanoflow"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t)
			defer h.close()

			target := kind + " MyFirstModule.ChangeLists"
			script := "mdl 1;\ncreate " + target + " (\n" +
				"  $Others: List of FeedbackModule.Feedback,\n" +
				"  $One: FeedbackModule.Feedback\n)\nbegin\n" +
				"  $Feedbacks = create list of FeedbackModule.Feedback;\n  " +
				strings.Join(rows, "\n  ") + "\nend;\n"
			if err := h.exec(script); err != nil {
				t.Fatalf("exec: %v\n%s", err, h.out.String())
			}
			// The stored actions: four Change list actions, one per operation,
			// and no Change variable action (on a list that is CE7247).
			unit := h.flowUnit(t, "ChangeLists")
			var ops []string
			for _, m := range changeListTypeRe.FindAllSubmatch(unit, -1) {
				ops = append(ops, string(m[1]))
			}
			if got := strings.Join(ops, ","); got != "Add,Remove,Clear,Set" {
				t.Errorf("stored Change list operations = %s, want Add,Remove,Clear,Set", got)
			}
			if bytes.Contains(unit, []byte("Microflows$ChangeVariableAction")) {
				t.Errorf("a Change variable action was written for `set` on a list (CE7247)")
			}

			first := h.describeUnder("mdl 1;", target)
			body := positionLine.ReplaceAllString(first, "")
			for _, row := range rows {
				if !strings.Contains(body, "  "+row+"\n") {
					t.Errorf("describe does not print %q:\n%s", row, body)
				}
			}

			// GetPut: executing the description writes nothing.
			if err := h.exec("mdl 1;\n" + first); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, first)
			}
			if !strings.Contains(strings.ToLower(h.out.String()), "unchanged "+kind+": myfirstmodule.changelists") {
				t.Errorf("GetPut: the description of an unchanged %s must write nothing, got:\n%s", kind, h.out.String())
			}
			// PutGet: describing again returns the same text.
			if again := h.describeUnder("mdl 1;", target); again != first {
				t.Errorf("describe -> exec -> describe changed the %s:\n%s", kind, lineDiff(first, again))
			}
		})
	}
}
