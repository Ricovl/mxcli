// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Settings a Studio Pro author can give a workflow that MDL has no words for, so
// a rebuild from a statement resets them without a word (ako/mxcli#1713).
// Measured on mxbuild 11.15.0: a stored workflow given each of them passed mx
// check; describe printed none of them, and replaying the description with
// `create or modify` reported "Modified workflow" and left none of them:
//
//	a user task's task name 'Review {1}' (caption 'Review')   reset to 'Review'
//	its parameter {1} = $WorkflowContext/Name                 dropped
//	a timer boundary event's caption                          blanked
//	a non-interrupting timer's recurrence                     dropped (a one-shot reminder)
//
// The rebuild takes a user task's name from its caption, writes a template's
// text without its parameters, and writes a timer boundary event with neither a
// caption nor a recurrence. Each is refused until MDL can state it — the issue
// named after each reason adds the syntax.

// workflowTemplateLabel names a stored string template by its key, as MDL calls
// the clause that states its text.
var workflowTemplateLabel = map[string]string{
	"WorkflowName":        "display",
	"WorkflowDescription": "description",
	"TaskName":            "task name",
	"TaskDescription":     "description",
}

// rawTemplateParams describes a stored string template whose placeholders are
// filled from expressions — `'Review {1}' with ({1} = $WorkflowContext/Name)` —
// or returns "" when it has none.
func rawTemplateParams(v any) string {
	st, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	var params []string
	for _, p := range rawList(st["Parameters"]) {
		if pm, ok := p.(map[string]any); ok {
			expr, _ := pm["Expression"].(string)
			params = append(params, fmt.Sprintf("{%d} = %s", len(params)+1, expr))
		}
	}
	if len(params) == 0 {
		return ""
	}
	text, _ := st["Text"].(string)
	return fmt.Sprintf("'%s' with (%s)", text, strings.Join(params, ", "))
}

// rawWorkflowTemplateLosses lists the workflow's own templates whose parameters
// a rebuild drops.
func rawWorkflowTemplateLosses(raw map[string]any) []string {
	var out []string
	for _, k := range []string{"WorkflowName", "WorkflowDescription"} {
		if p := rawTemplateParams(raw[k]); p != "" {
			out = append(out, fmt.Sprintf("the workflow's %s %s fills its placeholders from expressions, which MDL cannot state yet (ako/mxcli#1714)",
				workflowTemplateLabel[k], p))
		}
	}
	return out
}

func isRawUserTask(d map[string]any) bool {
	switch t, _ := d["$Type"].(string); t {
	case "Workflows$SingleUserTaskActivity", "Workflows$MultiUserTaskActivity", "Workflows$UserTask":
		return true
	}
	return false
}

// rawUserTaskTemplateLosses lists what rebuilding a stored user task from a
// statement resets in its task name and description; caption is the caption the
// statement gives it, which the rebuild takes the task name from.
func rawUserTaskTemplateLosses(d map[string]any, caption string) []string {
	var out []string
	if tn, ok := d["TaskName"].(map[string]any); ok {
		if text, _ := tn["Text"].(string); text != "" && text != caption {
			out = append(out, fmt.Sprintf("its task name '%s' is not its caption '%s', and the rebuilt task takes its name from the caption (ako/mxcli#1714)",
				text, caption))
		}
	}
	for _, k := range []string{"TaskName", "TaskDescription"} {
		if p := rawTemplateParams(d[k]); p != "" {
			out = append(out, fmt.Sprintf("its %s %s fills its placeholders from expressions, which MDL cannot state yet (ako/mxcli#1714)",
				workflowTemplateLabel[k], p))
		}
	}
	return out
}

// rawBoundaryEventLosses lists what rebuilding a stored activity resets on its
// boundary events: a timer's caption and recurrence, and an annotation.
func rawBoundaryEventLosses(d map[string]any) []string {
	var out []string
	for _, be := range rawList(d["BoundaryEvents"]) {
		bm, ok := be.(map[string]any)
		if !ok {
			continue
		}
		kind := rawBoundaryEventKind(bm)
		if strings.HasSuffix(kind, "timer") {
			if c, _ := bm["Caption"].(string); c != "" {
				out = append(out, fmt.Sprintf("its %s boundary event has caption '%s', which MDL cannot state for a timer yet (ako/mxcli#1715)", kind, c))
			}
		}
		if r, ok := bm["Recurrence"].(map[string]any); ok {
			out = append(out, fmt.Sprintf("its %s boundary event repeats (every %v %v, at most %v times), which MDL cannot state yet (ako/mxcli#1716)",
				kind, r["Interval"], r["IntervalType"], r["MaxExecutions"]))
		}
		if a, ok := bm["Annotation"].(map[string]any); ok {
			text, _ := a["Description"].(string)
			out = append(out, fmt.Sprintf("its %s boundary event has annotation '%s', which MDL cannot state", kind, text))
		}
	}
	return out
}

// rawBoundaryEventKind is a stored boundary event's kind as MDL writes it after
// `boundary event`: "interrupting timer", "non interrupting notification", ….
func rawBoundaryEventKind(bm map[string]any) string {
	t, _ := bm["$Type"].(string)
	t = strings.TrimSuffix(strings.TrimPrefix(t, "Workflows$"), "BoundaryEvent")
	kind := "interrupting"
	if strings.HasPrefix(t, "NonInterrupting") {
		kind = "non interrupting"
		t = strings.TrimPrefix(t, "NonInterrupting")
	} else {
		t = strings.TrimPrefix(t, "Interrupting")
	}
	return kind + " " + strings.ToLower(t)
}

// rawUnstatableWorkflowState lists, for CREATE OR MODIFY WORKFLOW, what the
// stored workflow holds that rebuilding it from stmt resets. A user task's name
// and description count only when the statement restates the task — dropping
// the task drops them on purpose — while boundary-event state counts wherever it
// is, since a boundary event the statement does not restate is refused already.
func rawUnstatableWorkflowState(raw map[string]any, stmt *ast.CreateWorkflowStmt) []string {
	out := rawWorkflowTemplateLosses(raw)
	captions := map[string]string{} // user task name -> the caption the statement gives it
	walkWorkflowActivities(workflowStatementActivities(stmt), func(a ast.WorkflowActivityNode) {
		if t, ok := a.(*ast.WorkflowUserTaskNode); ok {
			captions[t.Name] = t.Caption
		}
	})
	walkRawDocs(raw, func(d map[string]any) {
		name, _ := d["Name"].(string)
		var reasons []string
		if caption, restated := captions[name]; restated && isRawUserTask(d) {
			reasons = append(reasons, rawUserTaskTemplateLosses(d, caption)...)
		}
		reasons = append(reasons, rawBoundaryEventLosses(d)...)
		for _, r := range reasons {
			out = append(out, fmt.Sprintf("activity '%s': %s", name, r))
		}
	})
	return out
}

// rawUnstatableActivityState lists, for REPLACE ACTIVITY, what the stored
// activity holds that rebuilding it from the replacement resets.
func rawUnstatableActivityState(d map[string]any, replacement ast.WorkflowActivityNode) []string {
	var out []string
	if isRawUserTask(d) {
		caption := ""
		if t, ok := replacement.(*ast.WorkflowUserTaskNode); ok {
			caption = t.Caption
		}
		out = append(out, rawUserTaskTemplateLosses(d, caption)...)
	}
	return append(out, rawBoundaryEventLosses(d)...)
}
