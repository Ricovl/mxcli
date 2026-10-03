// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// `set task outcome` takes the outcome as a literal, by design: Mendix stores
// Microflows$SetTaskOutcomeAction.Outcome as a by-name reference to one user
// task outcome, resolved when the model is built, so there is no expression to
// put a variable in. Written with a variable it never parsed, and the error
// said only "expecting STRING_LITERAL" (ako/mxcli#944, item 5). It now says
// why, and how to write a shared claim-and-complete flow. Under every language
// version, since none of them ever parsed it.
func TestSetTaskOutcomeVariableIsExplained(t *testing.T) {
	body := "create microflow M.Complete ($Task: System.WorkflowUserTask, $Outcome: String) begin\n" +
		"  set task outcome $Task %s;\nend;"
	for _, header := range []string{"", "mdl 0;\n", "mdl 1;\n"} {
		name := strings.TrimSpace(header)
		if name == "" {
			name = "headerless"
		}
		t.Run(name, func(t *testing.T) {
			control := header + strings.Replace(body, "%s", "'Approve'", 1)
			if _, errs := Build(control); len(errs) > 0 {
				t.Fatalf("control does not parse: %v", errs[0])
			}
			_, errs := Build(header + strings.Replace(body, "%s", "$Outcome", 1))
			if len(errs) == 0 {
				t.Fatal("set task outcome with a variable parsed")
			}
			all := ""
			for _, e := range errs {
				all += e.Error() + "\n"
			}
			for _, want := range []string{"by name", "one branch per outcome", "set task outcome $Task 'Approve';"} {
				if !strings.Contains(all, want) {
					t.Errorf("the parse error does not say %q:\n%s", want, all)
				}
			}
		})
	}
}
