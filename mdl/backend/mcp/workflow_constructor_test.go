// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Studio Pro 11.14's constructor, as ped_get_schema returned it live (trimmed to
// the lines the probe reads).
const workflowCtorSchema1114 = `{"kind":"constructor","schema":"constructor type 'Workflows$Workflow' = {\n $Type: 'Workflows$Workflow',\n name: string;\n flow?: Element<'Workflows$Flow'>;\n context: Reference<'DomainModels$Entity', 'qualified-name'>;\n onWorkflowEvent?: Element<'Workflows$WorkflowEventHandler'>[];\n caption?: string;\n workflowName?: string;\n}"}`

// The shape is chosen from the live constructor schema, once per session: 11.14
// takes `context` and rejects a `parameter` element, and the release that changed
// it is not known, so a version gate would be a guess.
func TestWorkflowConstructorShapeProbe(t *testing.T) {
	schemaCalls := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_get_schema" {
			schemaCalls++
			return workflowCtorSchema1114, false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	if !b.workflowConstructorTakesContext() || !b.workflowConstructorTakesContext() {
		t.Fatal("an 11.14 constructor schema must select the context shape")
	}
	if schemaCalls != 1 {
		t.Errorf("ped_get_schema called %d times, want 1 (cached)", schemaCalls)
	}
	if !b.schemaFetched[workflowDocType] {
		t.Error("the probe must count as the schema fetch PED requires before a create")
	}

	old := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return `{"schemas":[{"elementType":"Workflows$Workflow","schema":{"properties":{"parameter":{}}}}]}`, false
	})
	if (&Backend{client: old.connectClient(t)}).workflowConstructorTakesContext() {
		t.Error("a constructor without `context` must keep the parameter shape")
	}
}

func TestMapWorkflowContent_ContextShape(t *testing.T) {
	b := &Backend{}
	wf := &workflows.Workflow{
		Name:         "Approve",
		WorkflowName: "Approve Order",
		Parameter:    &workflows.WorkflowParameter{EntityRef: "M.OrderCtx"},
		EventHandlers: []*workflows.WorkflowEventHandler{{
			Description: "Audit", EventTypes: []string{"WorkflowCompleted"}, Microflow: "M.ACT_Audit",
		}},
	}
	content, err := b.mapWorkflowContent(wf, true)
	if err != nil {
		t.Fatal(err)
	}
	if content["context"] != "M.OrderCtx" || content["workflowName"] != "Approve Order" || content["caption"] != "Approve Order" {
		t.Errorf("context shape = %+v", content)
	}
	for _, key := range []string{"parameter", "title", "workflowDescription", "documentation", "excluded", "workflowV2"} {
		if _, ok := content[key]; ok {
			t.Errorf("the 11.14 constructor does not take %q, but it was sent", key)
		}
	}
	if hs, _ := content["onWorkflowEvent"].([]any); len(hs) != 1 {
		t.Errorf("onWorkflowEvent = %v", content["onWorkflowEvent"])
	}

	legacy, err := b.mapWorkflowContent(wf, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy["parameter"]; !ok {
		t.Error("the parameter shape must still send `parameter`")
	}
}

// Studio Pro 11.15's constructor, as ped_get_schema returned it live (trimmed):
// `caption` became `title`. 11.15 schemas are additionalProperties-permissive, so
// a `caption` sent to it is accepted and dropped — the workflow is created
// without its title, silently.
const workflowCtorSchema1115 = `{"kind":"constructor","schema":"constructor type 'Workflows$Workflow' = {\n $Type: 'Workflows$Workflow',\n name: string;\n flow?: Element<'Workflows$Flow'>;\n context: Reference<'DomainModels$Entity', 'qualified-name'>;\n eventSubProcesses: Element<'Workflows$EventSubProcess'>[] = [];\n onWorkflowEvent?: Element<'Workflows$WorkflowEventHandler'>[];\n // The title of the workflow, in the user interface known as 'caption'\n title?: string;\n workflowName?: string;\n}"}`

func TestMapWorkflowContent_ContextShapeTitle1115(t *testing.T) {
	for _, tc := range []struct {
		schema, want, notWant string
	}{
		{workflowCtorSchema1115, "title", "caption"},
		{workflowCtorSchema1114, "caption", "title"},
	} {
		f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
			if name == "ped_get_schema" {
				return tc.schema, false
			}
			return "SUCCESS", false
		})
		b := &Backend{client: f.connectClient(t)}
		if !b.workflowConstructorTakesContext() {
			t.Fatal("both constructors take `context`")
		}
		wf := &workflows.Workflow{Name: "Approve", WorkflowName: "Approve Order", Parameter: &workflows.WorkflowParameter{EntityRef: "M.OrderCtx"}}
		content, err := b.mapWorkflowContent(wf, true)
		if err != nil {
			t.Fatal(err)
		}
		if content[tc.want] != "Approve Order" {
			t.Errorf("%s constructor: %q = %v, want the title (content %+v)", tc.want, tc.want, content[tc.want], content)
		}
		if _, ok := content[tc.notWant]; ok {
			t.Errorf("%s constructor: %q sent, which it does not take", tc.want, tc.notWant)
		}
	}
}

// Neither constructor applies the title or the workflow name it is given:
// measured live, 11.14 drops `caption`/`workflowName` and 11.15 drops
// `title`/`workflowName` — both store the document name in both — so DISPLAY was
// lost over MCP. They are set as leaves after the create, which both accept.
func TestWorkflowCreateLeafOps_SetsTitleAndWorkflowName(t *testing.T) {
	wf := &workflows.Workflow{Name: "Approve", WorkflowName: "Approve Order", Documentation: "doc", WorkflowDescription: "desc"}
	got := map[string]any{}
	for _, op := range workflowCreateLeafOps(wf) {
		got[op.Path] = op.Operation.Value
	}
	want := map[string]any{
		"/title":                    "Approve Order",
		"/workflowName/text":        "Approve Order",
		"/documentation":            "doc",
		"/workflowDescription/text": "desc",
	}
	for p, v := range want {
		if got[p] != v {
			t.Errorf("%s = %v, want %v (ops %v)", p, got[p], v, got)
		}
	}
	if ops := workflowCreateLeafOps(&workflows.Workflow{Name: "Approve"}); len(ops) != 0 {
		t.Errorf("no DISPLAY: the constructor's defaults (the name) are right, want no ops, got %v", ops)
	}
}
