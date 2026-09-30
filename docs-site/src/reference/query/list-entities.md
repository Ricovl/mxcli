# LIST ENTITIES

## Synopsis

    LIST ENTITIES [IN <module>]

## Description

Lists entities in the project. Without the `IN` clause, lists all entities across all modules. With `IN <module>`, restricts the listing to entities in the specified module.

`SHOW ENTITY <qualified_name>` printed a summary of one entity. It has no replacement that prints the same thing: use `DESCRIBE ENTITY` for the definition, or this listing for the summary columns. Without a language header it still runs and warns `MDL-V1-SHOWSUMMARY`; under `mdl 1;` it is an error.

## Parameters

*module*
: The name of the module to filter by. Only entities belonging to this module are shown.

## Examples

List all entities in the project:

```sql
LIST ENTITIES
```

List entities in a specific module:

```sql
LIST ENTITIES IN Sales
```

Show the definition of a single entity:

```sql
DESCRIBE ENTITY Sales.Customer
```

## See Also

[DESCRIBE ENTITY](describe-entity.md), [LIST ASSOCIATIONS](list-associations.md), [LIST MODULES](list-modules.md)
