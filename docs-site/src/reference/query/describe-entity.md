# DESCRIBE ENTITY

## Synopsis

    DESCRIBE ENTITY <qualified_name>

## Description

Shows the complete MDL source for an entity, including its type (persistent, non-persistent, view, external), all attributes with types and constraints, indexes, access rules, and documentation comments. The output is round-trippable MDL that can be used directly in `CREATE ENTITY` statements.

This is the primary way to inspect the full definition of an entity before modifying it with `ALTER ENTITY` or recreating it with `CREATE OR MODIFY`.

## Parameters

*qualified_name*
: A `Module.EntityName` reference identifying the entity to describe. Both the module and entity name are required.

## Examples

Describe a customer entity:

```sql
DESCRIBE ENTITY Sales.Customer
```

Example output:

```sql
/**
 * Customer entity stores customer information.
 */
@Position(100, 200)
CREATE PERSISTENT ENTITY Sales.Customer (
  /** Customer identifier */
  CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
  Name: String(200) NOT NULL,
  Email: String(200),
  Status: Enumeration(Sales.CustomerStatus) DEFAULT 'Active'
)
INDEX (Name);
```

Describe a non-persistent entity:

```sql
DESCRIBE ENTITY Sales.CustomerFilter
```

## See Also

[LIST ENTITIES](list-entities.md), [DESCRIBE ASSOCIATION](describe-association.md), [DESCRIBE ENUMERATION](describe-enumeration.md)
