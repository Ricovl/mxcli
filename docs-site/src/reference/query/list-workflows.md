# LIST WORKFLOWS

## Synopsis

    LIST WORKFLOWS [IN <module>]

## Description

Lists workflows defined in the project. Without the `IN` clause, lists all workflows across all modules. With `IN <module>`, restricts the listing to workflows in the specified module.

Workflows model multi-step business processes with user tasks, decisions, parallel paths, and microflow calls.

## Parameters

*module*
: The name of the module to filter by. Only workflows belonging to this module are shown.

## Examples

List all workflows in the project:

```sql
LIST WORKFLOWS
```

List workflows in a specific module:

```sql
LIST WORKFLOWS IN Approvals
```

## See Also

[LIST MODULES](list-modules.md), [LIST MICROFLOWS](list-microflows.md), [DESCRIBE STRUCTURE](describe-structure.md)
