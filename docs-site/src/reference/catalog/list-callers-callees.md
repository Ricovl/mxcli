# LIST CALLERS / CALLEES

## Synopsis

    LIST CALLERS OF qualified_name [ TRANSITIVE ]

    LIST CALLEES OF qualified_name [ TRANSITIVE ]

## Description

Displays the call relationships for a given element. `LIST CALLERS` finds all elements that call or reference the specified element (incoming edges). `LIST CALLEES` finds all elements that the specified element calls or references (outgoing edges).

These commands require `REFRESH CATALOG FULL` to have been run beforehand. Without it, the cross-reference data needed for caller/callee analysis is not available.

By default, only direct (one-hop) relationships are shown. With the `TRANSITIVE` option, the command follows the call chain recursively to show the full transitive closure -- all indirect callers or callees at any depth.

## Parameters

**qualified_name**
: The fully qualified name of the element to analyze (e.g., `Module.MicroflowName`, `Module.EntityName`). Works with microflows, nanoflows, pages, snippets, entities, and other referenceable elements.

**TRANSITIVE**
: Follow the call chain recursively. For `LIST CALLERS`, this finds everything that directly or indirectly leads to the target. For `LIST CALLEES`, this finds everything the target directly or indirectly depends on.

## Examples

### Find direct callers of a microflow

```sql
REFRESH CATALOG FULL;
LIST CALLERS OF Sales.ACT_CreateOrder;
```

### Find all transitive callers

```sql
LIST CALLERS OF Sales.ACT_CreateOrder TRANSITIVE;
```

### Find what a microflow calls

```sql
LIST CALLEES OF Sales.ACT_ProcessOrder;
```

### Find all transitive callees

```sql
LIST CALLEES OF Sales.ACT_ProcessOrder TRANSITIVE;
```

### From the command line

```sql
-- Shell commands:
-- mxcli callers -p app.mpr Sales.ACT_CreateOrder
-- mxcli callers -p app.mpr Sales.ACT_CreateOrder --transitive
-- mxcli callees -p app.mpr Sales.ACT_ProcessOrder
```

## See Also

[LIST REFERENCES / IMPACT / CONTEXT](list-references-impact.md), [REFRESH CATALOG](refresh-catalog.md), [SELECT FROM CATALOG](select-from-catalog.md)
