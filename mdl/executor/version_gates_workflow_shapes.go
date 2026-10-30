// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// statementTypeGate is a typeGate on what a whole statement states rather than
// on an activity in a flow body: applies reports a statement that writes its
// types and properties.
type statementTypeGate struct {
	typeGate
	applies func(ast.Statement) bool
}

// workflowShapeGates are the gates of what a workflow statement states whose
// floor is above the workflow document's own (9.0.2, workflowBasicGate): the
// types and properties mxcli writes for them with no older shape
// (ako/mxcli#1811). Measured on stamped 9.x projects, a workflow with a
// parameter is refused below 9.18 (Workflows$Parameter.Name) and one with a
// user task below 9.19 (Workflows$UserTaskOutcome.Value), so most workflows
// meet those two first. Newest first.
var workflowShapeGates = []statementTypeGate{
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "interrupting_notification_event_subprocess",
				"event subprocess … on interrupting notification",
				"this version has no interrupting notification start (Workflows$InterruptingNotificationEventSubProcessStartActivity): start it with a non-interrupting notification, or upgrade the project"},
			types: []string{"Workflows$InterruptingNotificationEventSubProcessStartActivity"},
		},
		applies: func(stmt ast.Statement) bool {
			s, ok := stmt.(*ast.CreateWorkflowStmt)
			if !ok {
				return false
			}
			for _, esp := range s.EventSubProcesses {
				if esp.Interrupting && !esp.Timer {
					return true
				}
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "user_task", "user task",
				"this version stores a user task differently (its outcome's value, Workflows$UserTaskOutcome.Value, and task page), which mxcli does not write: create it in Studio Pro, or upgrade the project"},
			types:      []string{"Workflows$NoEvent", "Workflows$MicroflowBasedEvent", "Workflows$PageReference"},
			properties: []string{"Workflows$UserTask.OnCreatedEvent", "Workflows$UserTask.TaskPage", "Workflows$UserTaskOutcome.Value"},
		},
		applies: func(stmt ast.Statement) bool {
			return workflowStatesActivity(stmt, func(a ast.WorkflowActivityNode) bool { _, ok := a.(*ast.WorkflowUserTaskNode); return ok })
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "workflow_parameter", "parameter $WorkflowContext: …",
				"this version stores a workflow's parameter differently (Workflows$Parameter and its Name), which mxcli does not write: create the workflow in Studio Pro, or upgrade the project"},
			types:      []string{"Workflows$Parameter"},
			properties: []string{"Workflows$Workflow.Parameter", "Workflows$Parameter.Entity", "Workflows$Parameter.Name"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreateWorkflowStmt:
				return s.ParameterVar != "" || s.ParameterEntity.Name != ""
			case *ast.AlterWorkflowStmt:
				return alterWorkflowSets(s, "PARAMETER")
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "call_workflow_parameter_mappings", "call workflow … ( mappings ) (an activity)",
				"this version has no parameter mappings on a call workflow activity (Workflows$WorkflowCallParameterMapping): pass the context object, or upgrade the project"},
			types:      []string{"Workflows$WorkflowCallParameterMapping"},
			properties: []string{"Workflows$CallWorkflowActivity.ParameterMappings"},
		},
		applies: func(stmt ast.Statement) bool {
			return workflowStatesActivity(stmt, func(a ast.WorkflowActivityNode) bool {
				n, ok := a.(*ast.WorkflowCallWorkflowNode)
				return ok && len(n.ParameterMappings) > 0
			})
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "annotations", "an annotation in a workflow",
				"this version has no workflow annotations (Workflows$Annotation): keep the note as an MDL comment, or upgrade the project"},
			types:      []string{"Workflows$Annotation"},
			properties: []string{"Workflows$Workflow.Annotation"},
		},
		applies: func(stmt ast.Statement) bool {
			if s, ok := stmt.(*ast.CreateWorkflowStmt); ok {
				if s.Annotation != "" {
					return true
				}
				for _, esp := range s.EventSubProcesses {
					if esp.Annotation != "" {
						return true
					}
				}
			}
			return workflowStatesActivity(stmt, func(a ast.WorkflowActivityNode) bool {
				if _, ok := a.(*ast.WorkflowAnnotationActivityNode); ok {
					return true
				}
				an, ok := a.(ast.AnnotatedWorkflowActivity)
				return ok && an.ActivityAnnotation() != ""
			})
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "overview_page", "overview page (of a workflow)",
				"this version stores a workflow's overview page differently (Workflows$Workflow.AdminPage), which mxcli does not write: set it in Studio Pro, or upgrade the project"},
			types:      []string{"Workflows$PageReference"},
			properties: []string{"Workflows$Workflow.AdminPage"},
		},
		applies: func(stmt ast.Statement) bool {
			switch s := stmt.(type) {
			case *ast.CreateWorkflowStmt:
				return s.OverviewPage.Name != ""
			case *ast.AlterWorkflowStmt:
				return alterWorkflowSets(s, "OVERVIEW_PAGE")
			}
			return false
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "call_workflow_activity", "call workflow (an activity)",
				"this version stores a call workflow activity differently (Workflows$CallWorkflowActivity.ParameterExpression), which mxcli does not write: upgrade the project"},
			types:      []string{"Workflows$CallWorkflowActivity"},
			properties: []string{"Workflows$CallWorkflowActivity.ParameterExpression"},
		},
		applies: func(stmt ast.Statement) bool {
			return workflowStatesActivity(stmt, func(a ast.WorkflowActivityNode) bool { _, ok := a.(*ast.WorkflowCallWorkflowNode); return ok })
		},
	},
	{
		typeGate: typeGate{
			versionGate: versionGate{"workflows", "jump_to", "jump to",
				"this version has no jump activity (Workflows$JumpToActivity): upgrade the project"},
			types: []string{"Workflows$JumpToActivity"},
		},
		applies: func(stmt ast.Statement) bool {
			return workflowStatesActivity(stmt, func(a ast.WorkflowActivityNode) bool { _, ok := a.(*ast.WorkflowJumpToNode); return ok })
		},
	},
}

// workflowStatesActivity reports an activity a create or alter workflow writes
// that match accepts, nested and event sub-process ones included.
func workflowStatesActivity(stmt ast.Statement, match func(ast.WorkflowActivityNode) bool) bool {
	var activities []ast.WorkflowActivityNode
	switch s := stmt.(type) {
	case *ast.CreateWorkflowStmt:
		activities = workflowStatementActivities(s)
	case *ast.AlterWorkflowStmt:
		activities = alterWorkflowAddedActivities(s)
	default:
		return false
	}
	found := false
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		found = found || match(a)
	})
	return found
}

// alterWorkflowSets reports an `alter workflow … set (Property: …)`.
func alterWorkflowSets(s *ast.AlterWorkflowStmt, property string) bool {
	for _, op := range s.Operations {
		if o, ok := op.(*ast.SetWorkflowPropertyOp); ok && strings.EqualFold(o.Property, property) {
			return true
		}
	}
	return false
}

// workflowShapeGatesOf are the workflowShapeGates stmt states.
func workflowShapeGatesOf(stmt ast.Statement) []versionGate {
	var gates []versionGate
	for _, g := range workflowShapeGates {
		if g.applies(stmt) {
			gates = append(gates, g.versionGate)
		}
	}
	return gates
}
