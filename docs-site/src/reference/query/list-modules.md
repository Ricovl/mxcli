# LIST MODULES

## Synopsis

    LIST MODULES

## Description

Lists all modules in the current project with their names. This is typically the first command used after connecting to a project to understand its structure.

Module names returned by this command are used as the `IN <module>` filter for other `SHOW` and `DESCRIBE` statements.

## Parameters

This statement takes no parameters.

## Examples

List all modules in the project:

```sql
LIST MODULES;
```

Use the result to explore a specific module:

```sql
LIST MODULES;
LIST ENTITIES IN MyFirstModule;
```

## See Also

[DESCRIBE STRUCTURE](describe-structure.md), [LIST ENTITIES](list-entities.md), [LIST MICROFLOWS](list-microflows.md), [LIST PAGES](list-pages.md)
