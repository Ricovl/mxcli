// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL-WF19: a variable a workflow expression cannot see (mendixlabs/mxcli#1529,
// #1523). A text template's parameters are expressions too (ako/mxcli#1714):
// the workflow's display and description are header scope, a user task's task
// name and description activity scope — measured on 11.15.0.
//
// A workflow declares no variables. What an expression in it can name is fixed
// by Mendix, and anything else is CE0117 "Error(s) in expression." at build time
// — after `check --references` and `exec` had both passed and the model was
// written. Measured with `mx check` on 11.6.6 and on 11.15.0, verdicts identical,
// one probe per expression kind:
//
//	                          $WorkflowContext  $WorkflowInstance  anything else
//	call microflow argument          ok               ok              CE0117
//	call workflow argument           ok              (ok)             CE0117
//	decision condition               ok               ok              CE0117
//	wait for timer                   ok               ok              CE0117
//	user task due date               ok               ok              CE0117
//	boundary event timer             ok               ok              CE0117
//	event sub-process timer start    ok               ok              CE0117
//	the workflow's own due date      ok             CE0117            CE0117
//
// "Anything else" was measured with the names an author reaches for:
// `$Workflow` (what a microflow names its System.Workflow parameter — the
// report), `$currentUser`, `$currentSession` (a workflow runs with no user
// session) and `$WorkflowUserTask` (the task page's parameter, not in scope in
// the task's own due date or boundary timer). The call-workflow row's
// $WorkflowInstance was not probed directly; it is the same activity scope.
//
// Two spellings are accepted although Mendix's name differs, because the writer
// rewrites them: the header's declared name for the context and any casing of
// `$WorkflowContext` (contextExprNormalizer), and any casing of
// `$WorkflowInstance` (lower case is CE0117 as written, measured on 11.6.6).
//
// XPath targeting is not scanned here: it is an XPath constraint, not an
// expression, and validateWorkflowTargetingXPath checks its members instead.
const wfExprScopeRule = "MDL-WF19"

// workflowInstanceVar is the System.Workflow object of the running instance —
// the value a microflow's `Workflow: System.Workflow` parameter is passed.
const workflowInstanceVar = "WorkflowInstance"

// wfExprSite is one expression a workflow statement writes.
type wfExprSite struct {
	where string // what holds it, for the message
	expr  string
	// header marks the workflow's own due date, which is evaluated with only the
	// context in scope (no instance yet).
	header bool
}

var (
	wfExprStringRe = regexp.MustCompile(`'(?:[^']|'')*'`)
	wfExprVarRe    = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// workflowExprVars returns the `$names` an expression references, in order,
// ignoring anything inside a string literal.
func workflowExprVars(expr string) []string {
	stripped := wfExprStringRe.ReplaceAllString(expr, "''")
	var out []string
	for _, m := range wfExprVarRe.FindAllStringSubmatch(stripped, -1) {
		out = append(out, m[1])
	}
	return out
}

// workflowActivityExprSites collects the expressions the activities (and every
// nested flow) write.
func workflowActivityExprSites(acts []ast.WorkflowActivityNode) []wfExprSite {
	var sites []wfExprSite
	boundary := func(events []ast.WorkflowBoundaryEventNode, owner string) {
		for _, b := range events {
			if b.Delay != "" {
				sites = append(sites, wfExprSite{where: "the boundary event timer on " + owner, expr: b.Delay})
			}
		}
	}
	mappings := func(pms []ast.WorkflowParameterMappingNode, kind, callee string) {
		for _, pm := range pms {
			sites = append(sites, wfExprSite{
				where: fmt.Sprintf("%s %s, argument %s", kind, callee, pm.Parameter),
				expr:  pm.Expression,
			})
		}
	}
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		switch n := a.(type) {
		case *ast.WorkflowCallMicroflowNode:
			mappings(n.ParameterMappings, "call microflow", n.Microflow.String())
			boundary(n.BoundaryEvents, "call microflow "+n.Microflow.String())
		case *ast.WorkflowCallWorkflowNode:
			mappings(n.ParameterMappings, "call workflow", n.Workflow.String())
			boundary(n.BoundaryEvents, "call workflow "+n.Workflow.String())
		case *ast.WorkflowDecisionNode:
			sites = append(sites, wfExprSite{where: "decision " + workflowDecisionLabel(n), expr: n.Expression})
		case *ast.WorkflowWaitForTimerNode:
			sites = append(sites, wfExprSite{where: "wait for timer", expr: n.DelayExpression})
		case *ast.WorkflowUserTaskNode:
			label := workflowUserTaskLabel(n)
			if n.DueDate != "" {
				sites = append(sites, wfExprSite{where: "the due date of user task " + label, expr: n.DueDate})
			}
			boundary(n.BoundaryEvents, "user task "+label)
		case *ast.WorkflowWaitForNotificationNode:
			boundary(n.BoundaryEvents, "wait for notification")
		}
	})
	return sites
}

// ValidateWorkflowExpressionScope applies MDL-WF19 to a CREATE WORKFLOW.
func ValidateWorkflowExpressionScope(stmt *ast.CreateWorkflowStmt) []linter.Violation {
	sites := workflowActivityExprSites(workflowStatementActivities(stmt))
	for _, esp := range stmt.EventSubProcesses {
		if esp.Timer && esp.FirstExecutionTime != "" {
			sites = append(sites, wfExprSite{where: "the timer start of event sub-process " + esp.Name, expr: esp.FirstExecutionTime})
		}
	}
	if stmt.DueDate != "" {
		sites = append(sites, wfExprSite{where: "the workflow's due date", expr: stmt.DueDate, header: true})
	}
	sites = append(sites, workflowTemplateExprSites(workflowTemplateSites(stmt), isWorkflowHeaderTemplate)...)
	return checkWorkflowExprScope(sites, stmt.ParameterVar, workflowLocation(stmt.Name))
}

// alterWorkflowExprSites collects the expressions an ALTER WORKFLOW writes.
func alterWorkflowExprSites(stmt *ast.AlterWorkflowStmt) []wfExprSite {
	sites := workflowActivityExprSites(alterWorkflowAddedActivities(stmt))
	sites = append(sites, workflowTemplateExprSites(alterWorkflowTemplateSites(stmt), isWorkflowHeaderTemplate)...)
	for _, op := range stmt.Operations {
		switch o := op.(type) {
		case *ast.InsertBoundaryEventOp:
			if o.Delay != "" {
				sites = append(sites, wfExprSite{where: "the boundary event timer on " + o.ActivityRef, expr: o.Delay})
			}
		case *ast.SetWorkflowPropertyOp:
			if strings.EqualFold(o.Property, "due_date") && o.Value != "" {
				sites = append(sites, wfExprSite{where: "the workflow's due date", expr: o.Value, header: true})
			}
		case *ast.SetActivityPropertyOp:
			if strings.EqualFold(o.Property, "due_date") && o.Value != "" {
				sites = append(sites, wfExprSite{where: "the due date of " + o.ActivityRef, expr: o.Value})
			}
		}
	}
	return sites
}

// checkWorkflowExprScope reports each expression naming a variable outside the
// workflow scope, once per expression and name. declared is the header's
// context variable ("" in ALTER, which cannot see it).
func checkWorkflowExprScope(sites []wfExprSite, declared string, loc linter.Location) []linter.Violation {
	alias := strings.TrimPrefix(declared, "$")
	var out []linter.Violation
	for _, s := range sites {
		seen := map[string]bool{}
		for _, name := range workflowExprVars(s.expr) {
			if seen[name] {
				continue
			}
			seen[name] = true
			if strings.EqualFold(name, workflowContextVar) || (alias != "" && strings.EqualFold(name, alias)) {
				continue
			}
			isInstance := strings.EqualFold(name, workflowInstanceVar)
			if isInstance && !s.header {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:     wfExprScopeRule,
				Severity:   linter.SeverityError,
				Location:   loc,
				Message:    fmt.Sprintf("%s uses $%s, which is not in scope in a workflow expression — MxBuild rejects it (CE0117 \"Error(s) in expression.\")", s.where, name),
				Suggestion: workflowScopeSuggestion(name, s.header, isInstance),
			})
		}
	}
	return out
}

// workflowScopeSuggestion names the variable the author most likely meant.
func workflowScopeSuggestion(name string, header, isInstance bool) string {
	const scope = "A workflow expression can name only the context, $WorkflowContext (or the name the header declares for it), and the running workflow, $WorkflowInstance (System.Workflow)."
	switch {
	case isInstance && header:
		return "The workflow's own due date, display and description see only $WorkflowContext, not the running instance. Use an attribute of the context — for a due date, e.g. addDays([%CurrentDateTime%], 3)."
	case strings.EqualFold(name, "Workflow"):
		if header {
			return "Only $WorkflowContext is in scope in the workflow's own due date, display and description."
		}
		return "The running workflow is $WorkflowInstance, not $Workflow — pass it to a System.Workflow parameter as `(Workflow = $WorkflowInstance)`. " + scope
	case strings.EqualFold(name, "currentUser"), strings.EqualFold(name, "currentSession"):
		return "A workflow runs without a user session, so $" + name + " is not available. Store the user on the context object, or resolve it in a microflow. " + scope
	case strings.EqualFold(name, "WorkflowUserTask"):
		return "$WorkflowUserTask is the user task PAGE's parameter; a workflow expression cannot see it. " + scope
	}
	return scope
}
