# Workflow Statements

Statements for creating, inspecting, and dropping workflows.

Mendix workflows model long-running business processes with user tasks, decisions, parallel splits, and other activity types. A workflow is parameterized by a context entity that carries the data flowing through the process.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE WORKFLOW](create-workflow.md) | Define a workflow with activities, user tasks, decisions, and parallel paths |
| [DROP WORKFLOW](drop-workflow.md) | Remove a workflow from the project |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Show workflows | `LIST WORKFLOWS [IN module]` |
| Describe workflow | `DESCRIBE WORKFLOW module.Name` |
