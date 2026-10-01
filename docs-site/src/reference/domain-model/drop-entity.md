# DROP ENTITY

## Synopsis

    DROP ENTITY module.name

## Description

`DROP ENTITY` removes an entity from a module's domain model. The entity and all of its attributes, validation rules, indexes, and access rules are deleted.

Associations that reference the dropped entity are **not** automatically removed. Drop those separately with `DROP ASSOCIATION` before or after dropping the entity to avoid dangling references.

Use `LIST IMPACT OF Module.EntityName` to check which microflows, pages, and associations reference the entity before dropping it.

## Parameters

**module.name**
: The qualified name of the entity to remove, in the form `Module.EntityName`.

## Examples

### Drop a single entity

```sql
DROP ENTITY Sales.CustomerFilter;
```

### Drop entity after checking impact

```sql
mdl 1;
-- Check what references this entity
LIST IMPACT OF Sales.OldEntity;

-- Drop related association first
DROP ASSOCIATION Sales.OldEntity_Customer;

-- Then drop the entity
DROP ENTITY Sales.OldEntity;
```

## See Also

[CREATE ENTITY](create-entity.md), [ALTER ENTITY](alter-entity.md), [DROP ASSOCIATION](drop-association.md)
