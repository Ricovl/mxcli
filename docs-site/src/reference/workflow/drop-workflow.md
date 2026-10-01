# DROP WORKFLOW

## Synopsis

```sql
DROP WORKFLOW module.Name;
```

## Description

Removes a workflow from the project. The workflow is identified by its qualified name. If the workflow does not exist, an error is returned.

Any references to the dropped workflow (e.g., CALL WORKFLOW activities in other workflows, or navigation entries) will become broken and should be updated or removed.

## Parameters

`module.Name`
:   The qualified name of the workflow to drop (`Module.WorkflowName`).

## Examples

```sql
DROP WORKFLOW HR.ApprovalFlow;
```

## See Also

[CREATE WORKFLOW](create-workflow.md)
