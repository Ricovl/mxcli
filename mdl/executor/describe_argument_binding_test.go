// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#751 (R4): describe binds every argument as `Param = expression`
// and every text-template parameter as `with ({n} = expression)`. Each line
// must also parse back with no deprecation recorded.

func assertCanonicalMicroflowLine(t *testing.T, line string) {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.F ($O: M.E, $N: String) begin " + line + " end;")
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", line, errs[0])
	}
	if len(prog.Deprecations) > 0 {
		t.Errorf("%q records %s", line, prog.Deprecations[0].Code)
	}
}

func TestDescribe_ShowMessageTemplateArguments(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowMessageAction{
		Type:               microflows.MessageTypeWarning,
		Template:           &model.Text{Translations: map[string]string{"en_US": "Hi {1} {2}"}},
		TemplateParameters: []string{"$N", "$O/Name"},
		Blocking:           true,
	}
	got := e.formatAction(action, nil, nil)
	want := "show message 'Hi {1} {2}' type Warning with ({1} = $N, {2} = $O/Name) blocking;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	assertCanonicalMicroflowLine(t, got)
}

// Describe used to drop a validation feedback's template arguments altogether,
// so describe -> exec lost them.
func TestDescribe_ValidationFeedbackTemplateArguments(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ValidationFeedbackAction{
		ObjectVariable:     "O",
		AttributeName:      "M.E.Name",
		Template:           &model.Text{Translations: map[string]string{"en_US": "{1} is wrong"}},
		TemplateParameters: []string{"$N"},
	}
	got := e.formatAction(action, nil, nil)
	want := "validation feedback $O/Name message '{1} is wrong' with ({1} = $N);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	assertCanonicalMicroflowLine(t, got)
}

func TestDescribe_SendRestRequestArguments(t *testing.T) {
	action := &microflows.RestOperationCallAction{
		Operation:         "M.C.Get",
		ParameterMappings: []*microflows.RestParameterMapping{{Parameter: "M.C.Get.id", Value: "$N"}},
	}
	got := formatRestOperationCallAction(nil, action)
	if !strings.Contains(got, "with (id = $N)") {
		t.Errorf("got %q, want the argument as `id = $N`", got)
	}
	assertCanonicalMicroflowLine(t, strings.ReplaceAll(got, "\n", " "))
}

func TestDescribe_WorkflowCallArguments(t *testing.T) {
	bare := &workflows.CallMicroflowTask{
		Microflow:         "M.G",
		ParameterMappings: []*workflows.ParameterMapping{{Parameter: "M.G.Order", Expression: "$WorkflowContext"}},
	}
	bare.Name = "act1"
	bare.Caption = "Go"
	got := strings.Join(formatSingleActivity(bare, "  "), "\n")
	if !strings.Contains(got, "call microflow M.G(Order = $WorkflowContext) as act1 comment 'Go'") {
		t.Errorf("describe = %q", got)
	}
	assertCanonicalWorkflowLine(t, got, false)

	// A stored expression the MDL grammar cannot write bare keeps the string
	// form, which carries it byte for byte.
	odd := &workflows.CallWorkflowActivity{
		Workflow:          "M.Sub",
		ParameterMappings: []*workflows.ParameterMapping{{Parameter: "M.Sub.Order", Expression: "$WorkflowContext\n"}},
	}
	odd.Name = "callWorkflow1"
	odd.Caption = "Sub"
	got = strings.Join(formatSingleActivity(odd, "  "), "\n")
	if !strings.Contains(got, "comment 'Sub' with (Order = '") {
		t.Errorf("describe = %q", got)
	}
	prog := assertCanonicalWorkflowLine(t, got, true)
	cw := prog.Statements[0].(*ast.CreateWorkflowStmt).Activities[0].(*ast.WorkflowCallWorkflowNode)
	if len(cw.ParameterMappings) != 1 || cw.ParameterMappings[0].Expression != "$WorkflowContext\n" {
		t.Errorf("re-parsed mappings = %+v, want the stored expression unchanged", cw.ParameterMappings)
	}
}

func assertCanonicalWorkflowLine(t *testing.T, line string, legacy bool) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build("create workflow M.W parameter $WorkflowContext: M.E begin\n" + line + "\nend workflow;")
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", line, errs[0])
	}
	if got := len(prog.Deprecations) > 0; got != legacy {
		t.Errorf("%q: records a deprecation = %v, want %v", line, got, legacy)
	}
	return prog
}
