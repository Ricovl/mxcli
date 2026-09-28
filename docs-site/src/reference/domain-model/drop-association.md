# DROP ASSOCIATION

## Synopsis

    DROP ASSOCIATION module.name

## Description

`DROP ASSOCIATION` removes an association (relationship) between two entities from the domain model. The entities themselves are not affected -- only the relationship is removed.

After dropping an association, any microflows, pages, or access rules that traverse it will become invalid. Use `LIST IMPACT OF Module.AssociationName` to check references before dropping.

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
LIST IMPACT OF Sales.OldAssociation;
DROP ASSOCIATION Sales.OldAssociation;
```

## See Also

[CREATE ASSOCIATION](create-association.md), [DROP ENTITY](drop-entity.md)
