# LIST MICROFLOWS / LIST NANOFLOWS

## Synopsis

    LIST MICROFLOWS [IN <module>]

    LIST NANOFLOWS [IN <module>]

## Description

Lists microflows or nanoflows in the project. Without the `IN` clause, lists all microflows (or nanoflows) across all modules. With `IN <module>`, restricts the listing to the specified module.

Microflows run on the server side and can perform database operations, call external services, and execute Java actions. Nanoflows run on the client side and are used for offline-capable and low-latency logic.

## Parameters

*module*
: The name of the module to filter by. Only microflows or nanoflows belonging to this module are shown.

## Examples

List all microflows in the project:

```sql
LIST MICROFLOWS;
```

List microflows in a specific module:

```sql
LIST MICROFLOWS IN Administration;
```

List all nanoflows:

```sql
LIST NANOFLOWS;
```

List nanoflows in a specific module:

```sql
LIST NANOFLOWS IN MyFirstModule;
```

## See Also

[DESCRIBE MICROFLOW](describe-microflow.md), [LIST PAGES](list-pages.md), [LIST MODULES](list-modules.md)
