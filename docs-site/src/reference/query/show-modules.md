# SHOW MODULES

## Synopsis

    SHOW MODULES

## Description

Lists all modules in the current project with their names. This is typically the first command used after connecting to a project to understand its structure.

Module names returned by this command are used as the `IN <module>` filter for other `SHOW` and `DESCRIBE` statements.

## Parameters

This statement takes no parameters.

## Examples

List all modules in the project:

```sql
SHOW MODULES
```

Use the result to explore a specific module:

```sql
SHOW MODULES
SHOW ENTITIES IN MyFirstModule
```

## See Also

[DESCRIBE STRUCTURE](show-structure.md), [SHOW ENTITIES](show-entities.md), [SHOW MICROFLOWS](show-microflows.md), [SHOW PAGES](show-pages.md)
