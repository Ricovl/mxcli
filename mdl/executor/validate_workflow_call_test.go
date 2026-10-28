// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1530. A workflow has exactly one parameter, and Mendix stores
// it as WorkflowContext whatever variable name the header declares — so
// `call workflow M.WF_Child(Task = $WorkflowContext)` wrote the mapping key
// M.WF_Child.Task, which `check` and `exec` accepted and the build rejected:
// CE1613 "The selected parameter 'M.WF_Child.Task' no longer exists" (measured
// on 11.6.6 and 11.15.0).

const callWorkflowWrongKey = `create workflow M.WF_Parent
  parameter $WorkflowContext: M.Item
begin
  call workflow M.WF_Child(Task = $WorkflowContext);
end workflow;`

func callWorkflowRuleHits(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == callWorkflowParameterRule {
			out = append(out, v)
		}
	}
	return out
}

func TestValidateWorkflow_CallWorkflowParameterNotWorkflowContext(t *testing.T) {
	hits := callWorkflowRuleHits(ValidateWorkflow(parseWorkflowStmt(t, callWorkflowWrongKey)))
	if len(hits) != 1 {
		t.Fatalf("want 1 %s violation, got %d", callWorkflowParameterRule, len(hits))
	}
	v := hits[0]
	if v.Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error", v.Severity)
	}
	for _, want := range []string{"Task", "M.WF_Child.Task", "CE1613"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message does not name %q: %s", want, v.Message)
		}
	}
	if !strings.Contains(v.Suggestion, "WorkflowContext = $WorkflowContext") {
		t.Errorf("suggestion does not show the key that builds: %s", v.Suggestion)
	}
}

// Controls: the stored name, in any casing, and a call with no mapping at all.
func TestValidateWorkflow_CallWorkflowContextKeyClean(t *testing.T) {
	for _, call := range []string{
		"call workflow M.WF_Child(WorkflowContext = $WorkflowContext);",
		"call workflow M.WF_Child(workflowContext = $WorkflowContext);",
		"call workflow M.WF_Child;",
	} {
		src := strings.Replace(callWorkflowWrongKey, "call workflow M.WF_Child(Task = $WorkflowContext);", call, 1)
		if hits := callWorkflowRuleHits(ValidateWorkflow(parseWorkflowStmt(t, src))); len(hits) != 0 {
			t.Errorf("%s: unexpected %s: %s", call, callWorkflowParameterRule, hits[0].Message)
		}
	}
}

// The rule reaches a call nested in an outcome, and one ALTER WORKFLOW inserts.
func TestValidateWorkflow_CallWorkflowParameterNestedAndAlter(t *testing.T) {
	nested := `create workflow M.WF_Parent
  parameter $WorkflowContext: M.Item
begin
  decision '$WorkflowContext/Name != empty'
    outcomes
      true -> { call workflow M.WF_Child(Request = $WorkflowContext); }
      false -> { }
  ;
end workflow;`
	if hits := callWorkflowRuleHits(ValidateWorkflow(parseWorkflowStmt(t, nested))); len(hits) != 1 {
		t.Errorf("nested call: want 1 %s violation, got %d", callWorkflowParameterRule, len(hits))
	}

	prog := parseMDL(t, `alter workflow M.WF_Parent {
  insert after start { call workflow M.WF_Child(Request = $WorkflowContext); }
};`)
	alter, ok := prog.Statements[0].(*ast.AlterWorkflowStmt)
	if !ok {
		t.Fatalf("statement is %T", prog.Statements[0])
	}
	if hits := callWorkflowRuleHits(ValidateAlterWorkflow(alter)); len(hits) != 1 {
		t.Errorf("alter insert: want 1 %s violation, got %d", callWorkflowParameterRule, len(hits))
	}
}

// exec is reachable without check (-c, the REPL, --no-check), so it refuses too,
// before anything is written.
func TestExecCreateWorkflow_RefusesCallWorkflowParameterNotWorkflowContext(t *testing.T) {
	wrote := false
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	mb.CreateWorkflowFunc = func(*workflows.Workflow) error { wrote = true; return nil }
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateWorkflow(ctx, parseWorkflowStmt(t, callWorkflowWrongKey))
	if err == nil {
		t.Fatal("exec wrote a call workflow mapping keyed on a parameter that does not exist")
	}
	if !strings.Contains(err.Error(), callWorkflowParameterRule) || !strings.Contains(err.Error(), "WorkflowContext") {
		t.Errorf("refusal does not name the rule and the right key: %v", err)
	}
	if wrote {
		t.Error("the workflow was written before the refusal")
	}
}

// A casing variant of the stored name is accepted, so the key written must be
// the stored spelling, as for the context variable in expressions.
func TestBuildCallWorkflowActivity_NormalizesContextKeyCasing(t *testing.T) {
	act := buildCallWorkflowActivity(&ast.WorkflowCallWorkflowNode{
		Workflow:          ast.QualifiedName{Module: "M", Name: "WF_Child"},
		ParameterMappings: []ast.WorkflowParameterMappingNode{{Parameter: "workflowcontext", Expression: "$WorkflowContext"}},
	})
	if len(act.ParameterMappings) != 1 {
		t.Fatalf("mappings = %d, want 1", len(act.ParameterMappings))
	}
	if got := act.ParameterMappings[0].Parameter; got != "M.WF_Child.WorkflowContext" {
		t.Errorf("mapping key = %q, want M.WF_Child.WorkflowContext", got)
	}
}
