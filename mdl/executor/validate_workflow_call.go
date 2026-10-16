// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// callWorkflowParameterRule (MDL-WF18): a `call workflow` mapping keyed on
// anything but WorkflowContext.
//
// A workflow has exactly one parameter, and it is stored as WorkflowContext
// whatever variable name the header declares (`parameter $Task: M.Item` is an
// alias, see contextExprNormalizer). The mapping key is written as
// <called workflow>.<key>, so `call workflow M.WF_Child(Task = $WorkflowContext)`
// stored M.WF_Child.Task — passed check and exec, then failed the build.
// Measured, one parent + child per blank project, verdict = the mx check line:
//
//	(Task = $WorkflowContext)             11.6.6, 11.15.0   CE1613 "The selected parameter 'M.WF_Child.Task' no longer exists."
//	(WorkflowContext = $WorkflowContext)  11.6.6, 11.15.0   0 errors
//
// Refused rather than renamed: a key that names some other parameter is a
// misunderstanding worth stating, and the corrected spelling is what describe
// prints back (mendixlabs/mxcli#1530).
const callWorkflowParameterRule = "MDL-WF18"

// callWorkflowParameterViolations applies MDL-WF18 to every `call workflow` in
// activities, nested ones included.
func callWorkflowParameterViolations(activities []ast.WorkflowActivityNode, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		n, ok := a.(*ast.WorkflowCallWorkflowNode)
		if !ok {
			return
		}
		qn := n.Workflow.Module + "." + n.Workflow.Name
		for _, pm := range n.ParameterMappings {
			key := strings.TrimPrefix(strings.TrimSpace(pm.Parameter), "$")
			if strings.EqualFold(key, workflowContextVar) {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   callWorkflowParameterRule,
				Severity: linter.SeverityError,
				Location: loc,
				Message: fmt.Sprintf("call workflow %s maps parameter '%s', but a workflow has one parameter and Mendix always stores it as WorkflowContext — "+
					"'%s.%s' does not exist and the build fails (CE1613)", qn, key, qn, key),
				Suggestion: fmt.Sprintf("Key the mapping on WorkflowContext: `call workflow %s(WorkflowContext = $WorkflowContext)`. "+
					"A name declared in a workflow's `parameter $Name: …` header is only a local alias for it.", qn),
			})
		}
	})
	return out
}
