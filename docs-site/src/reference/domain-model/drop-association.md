# DROP ASSOCIATION

## Synopsis

    DROP ASSOCIATION module.name

## Description

`DROP ASSOCIATION` removes an association (relationship) between two entities from the domain model. The entities themselves are not affected -- only the relationship is removed.

The access rules that named the association are reconciled: the entry is removed from the FROM entity's rules and, for an `OWNER Both` association, from the TO entity's — which for a cross-module association are in another module. A message definition that still exposes the association blocks the drop, naming the member to remove first.

Microflows and pages that traverse the association become invalid. Use `LIST IMPACT OF Module.AssociationName` to check references before dropping.

## Parameters

**module.name**
: The qualified name of the association to remove, in the form `Module.AssociationName`.

## Examples

### Drop an association

```sql
DROP ASSOCIATION Sales.Order_Customer;
```

### Drop after checking impact

```sql
mdl 1;
LIST IMPACT OF Sales.OldAssociation;
DROP ASSOCIATION Sales.OldAssociation;
```

## See Also

[CREATE ASSOCIATION](create-association.md), [DROP ENTITY](drop-entity.md)
