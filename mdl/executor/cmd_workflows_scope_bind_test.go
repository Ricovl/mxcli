// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1523: with no argument list, a workflow call-microflow bound
// EVERY parameter of its target to $WorkflowContext. A `Workflow: System.Workflow`
// parameter then received the context object — the wrong type — and the value
// it needs is $WorkflowInstance (measured 0 errors on 11.6.6 and 11.15.0 when
// passed explicitly).
func TestAutoBindCallMicroflow_SystemWorkflowParameter(t *testing.T) {
	mod := mkModule("WF")
	mf := mkMicroflow(mod.ID, "ACT_Bind")
	mf.Parameters = []*microflows.MicroflowParameter{
		{BaseElement: model.BaseElement{ID: nextID("param")}, Name: "Workflow",
			Type: &microflows.ObjectType{EntityQualifiedName: "System.Workflow"}},
		{BaseElement: model.BaseElement{ID: nextID("param")}, Name: "Item",
			Type: &microflows.ObjectType{EntityQualifiedName: "WF.Item"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	task := &workflows.CallMicroflowTask{Microflow: "WF.ACT_Bind"}
	task.ID = model.ID(nextID("act"))
	task.Name = "callMicroflow1"
	autoBindWorkflowParameters(ctx, []workflows.WorkflowActivity{task}, "WorkflowContext")

	got := map[string]string{}
	for _, pm := range task.ParameterMappings {
		got[pm.Parameter] = pm.Expression
	}
	want := map[string]string{
		"WF.ACT_Bind.Workflow": "$WorkflowInstance",
		"WF.ACT_Bind.Item":     "$WorkflowContext", // the control: unchanged
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("mapping %s = %q, want %q", p, got[p], w)
		}
	}
}

// MDL-WF19 accepts the declared header name and any casing of the two scope
// variables because the writer rewrites them. These are the expression sites the
// writer used to skip — a boundary timer, an event sub-process timer start and a
// call-workflow argument — so the header name reached Mendix verbatim (CE0117).
// The decision is the control: it was always rewritten.
func TestAutoBind_RewritesEveryWorkflowExpressionSite(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}))

	decision := &workflows.ExclusiveSplitActivity{Expression: "$Ctx/Ok and $workflowInstance/Name != empty"}
	decision.ID = model.ID(nextID("act"))
	decision.Name = "decision1"

	be := &workflows.BoundaryEvent{EventType: "InterruptingTimer", TimerDelay: "addDays($Ctx/Due, 1)", Flow: mkWfFlow()}
	be.ID = model.ID(nextID("be"))
	ut := &workflows.UserTask{BoundaryEvents: []*workflows.BoundaryEvent{be}}
	ut.ID = model.ID(nextID("act"))
	ut.Name = "userTask1"

	start := &workflows.EventSubProcessStartActivity{Timer: true, FirstExecutionTime: "addDays($workflowinstance/StartTime, 1)"}
	start.ID = model.ID(nextID("act"))
	start.Name = "espStart"

	pm := &workflows.ParameterMapping{Parameter: "WF.Sub.WorkflowContext", Expression: "$Ctx"}
	cw := &workflows.CallWorkflowActivity{Workflow: "WF.Sub", ParameterMappings: []*workflows.ParameterMapping{pm}}
	cw.ID = model.ID(nextID("act"))
	cw.Name = "callWorkflow1"

	autoBindWorkflowParameters(ctx, []workflows.WorkflowActivity{decision, ut, start, cw}, "$Ctx")

	checks := []struct{ where, got, want string }{
		{"decision (control)", decision.Expression, "$WorkflowContext/Ok and $WorkflowInstance/Name != empty"},
		{"boundary timer", be.TimerDelay, "addDays($WorkflowContext/Due, 1)"},
		{"event sub-process timer start", start.FirstExecutionTime, "addDays($WorkflowInstance/StartTime, 1)"},
		{"call workflow argument", pm.Expression, "$WorkflowContext"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.where, c.got, c.want)
		}
	}
}

// exec is reachable without check (-c, the REPL, --no-check), so MDL-WF19 is
// refused there too — otherwise the CE0117 model is written anyway (#833's reason).
func TestExecWorkflow_RefusesOutOfScopeVariable(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}))
	create := parseMDL(t, "create workflow M.W parameter $WorkflowContext: M.Item begin\n"+
		"  call microflow M.ACT_Bind(Workflow = $Workflow, Item = $WorkflowContext);\nend workflow;")
	err := execCreateWorkflow(ctx, create.Statements[0].(*ast.CreateWorkflowStmt))
	if err == nil || !strings.Contains(err.Error(), "$WorkflowInstance") {
		t.Errorf("create: err = %v, want a refusal naming $WorkflowInstance", err)
	}
	alter := parseMDL(t, "alter workflow M.W { insert after Review { call microflow M.ACT_Inst(Workflow = $Workflow); } };")
	err = execAlterWorkflow(ctx, alter.Statements[0].(*ast.AlterWorkflowStmt))
	if err == nil || !strings.Contains(err.Error(), "$WorkflowInstance") {
		t.Errorf("alter: err = %v, want a refusal naming $WorkflowInstance", err)
	}
}

// mendixlabs/mxcli#1523: the unmapped-parameter error suggested the deprecated
// `with (Workflow = ...)` (MDL-DEPR008) and nothing about WHAT to pass, and the
// author guessed $Workflow — CE0117. It now names the current argument form and,
// per parameter type, the value: $WorkflowInstance for System.Workflow, the
// context for the context entity.
func TestWorkflowParameterMappings_TypeAwareAdvice(t *testing.T) {
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	mf := &microflows.Microflow{Name: "ACT_Bind", Parameters: []*microflows.MicroflowParameter{
		{Name: "Workflow", Type: &microflows.ObjectType{EntityQualifiedName: "System.Workflow"}},
		{Name: "Item", Type: &microflows.ObjectType{EntityQualifiedName: "M.Item"}},
		{Name: "Note", Type: &microflows.StringType{}},
	}}
	mf.ContainerID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	s := parseWorkflowStmt(t, "create workflow M.W parameter $WorkflowContext: M.Item begin call microflow M.ACT_Bind; end workflow;")
	errs := validateWorkflowParameterMappings(ctx, s, nil)
	if len(errs) != 3 {
		t.Fatalf("got %d errors, want 3: %v", len(errs), errs)
	}
	want := []string{
		"call microflow M.ACT_Bind(Workflow = $WorkflowInstance)",
		"call microflow M.ACT_Bind(Item = $WorkflowContext)",
		"call microflow M.ACT_Bind(Note = ...)",
	}
	for i, w := range want {
		if !strings.Contains(errs[i], w) {
			t.Errorf("error %d = %q, want it to contain %q", i, errs[i], w)
		}
		if strings.Contains(errs[i], "with (") {
			t.Errorf("error %d still suggests the deprecated with (…) form: %q", i, errs[i])
		}
	}
}

// The header alias must not swallow a longer name that starts with it: a header
// `parameter $Workflow: …` rewrites $Workflow, never $WorkflowInstance.
func TestContextExprNormalizer_AliasPrefixOfInstance(t *testing.T) {
	n := newContextExprNormalizer("$Workflow")
	if got, want := n.rewrite("$Workflow/Title + $WorkflowInstance/Name"), "$WorkflowContext/Title + $WorkflowInstance/Name"; got != want {
		t.Errorf("rewrite = %q, want %q", got, want)
	}
}
