// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// ako/mxcli#1717: `alter workflow M.W set (Key: …);` without the braces failed
// to parse, and error recovery read `set (key: …)` as the session command `set
// <name> = <value>` — so the last error said a session command does not belong
// in a script, which sent the author nowhere near the fix.
func TestAlterWorkflowWithoutBracesIsHinted(t *testing.T) {
	_, errs := Build("mdl 1;\nalter workflow M.W set (Annotation: 'Started from the request form');\n")
	if len(errs) == 0 {
		t.Fatal("accepted")
	}
	var all []string
	for _, e := range errs {
		all = append(all, e.Error())
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "alter workflow M.W { set (Annotation:") {
		t.Errorf("no braces hint naming the statement:\n%s", joined)
	}
	if strings.Contains(joined, "session command") {
		t.Errorf("still diagnosed as a session command:\n%s", joined)
	}
}

// The controls: the braced form and the old per-action form parse, and a
// session command on a line of its own is still refused in a script.
func TestAlterWorkflowBracesHintControls(t *testing.T) {
	for _, src := range []string{
		"mdl 1;\nalter workflow M.W { set (Annotation: 'x'); };\n",
		"alter workflow M.W set display 'x';\n",
	} {
		if _, errs := Build(src); len(errs) != 0 {
			t.Errorf("%q: %v", src, errs)
		}
	}
	_, errs := Build("mdl 1;\nset autocommit = true;\n")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "session command") {
		t.Errorf("a session command in a script: %v", errs)
	}
}
