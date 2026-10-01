# DROP MICROFLOW / NANOFLOW

## Synopsis

```sql
mdl 1;
DROP MICROFLOW module.Name;

DROP NANOFLOW module.Name;
```

## Description

Removes a microflow or nanoflow from the project. The microflow or nanoflow must exist; otherwise an error is raised.

Dropping a microflow or nanoflow does not automatically update references to it from other microflows, pages, navigation, or security rules. Use `LIST IMPACT OF module.Name` before dropping to identify all references that will break.

## Parameters

`module.Name`
:   The qualified name of the microflow or nanoflow to drop (`Module.MicroflowName` or `Module.NanoflowName`).

## Examples

Drop a microflow:

```sql
DROP MICROFLOW Sales.ACT_CreateOrder;
```

Drop a nanoflow:

```sql
DROP NANOFLOW Sales.NAV_ValidateOrder;
```

Check impact before dropping:

```sql
mdl 1;
-- See what references this microflow
LIST IMPACT OF Sales.ACT_CreateOrder;

-- Then drop if safe
DROP MICROFLOW Sales.ACT_CreateOrder;
```

## See Also

[CREATE MICROFLOW](create-microflow.md), [CREATE NANOFLOW](create-nanoflow.md), [LIST IMPACT](/reference/catalog/list-references-impact.md)
