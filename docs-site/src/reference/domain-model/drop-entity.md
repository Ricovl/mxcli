# DROP ENTITY

## Synopsis

    DROP ENTITY module.name

## Description

`DROP ENTITY` removes an entity from a module's domain model. The entity and all of its attributes, validation rules, indexes, and access rules are deleted.

Every association with the dropped entity at either end is removed with it — including a cross-module association declared in another module. The access rules of the entities at the other end are reconciled afterwards, in whichever module they live, so no rule is left naming an association that no longer exists (Mendix would reject that with CE1613):

```
Reconciled 1 access rule(s) in module Shop after dropping the entity
Dropped entity: Crm.Contact
```

Microflows, pages and other documents that use the entity or one of those associations are not changed.

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

-- Drops the entity and its associations, and reconciles the access rules
DROP ENTITY Sales.OldEntity;
```

## See Also

[CREATE ENTITY](create-entity.md), [ALTER ENTITY](alter-entity.md), [DROP ASSOCIATION](drop-association.md)
