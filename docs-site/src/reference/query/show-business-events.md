# SHOW BUSINESS EVENTS

## Synopsis

    SHOW BUSINESS EVENTS [IN <module>]

## Description

Lists business event services defined in the project. Without the `IN` clause, lists all business event services across all modules. With `IN <module>`, restricts the listing to services in the specified module.

Business event services enable asynchronous, event-driven communication between Mendix applications using the Mendix Business Events platform.

## Parameters

*module*
: The name of the module to filter by. Only business event services belonging to this module are shown.

## Examples

List all business event services:

```sql
SHOW BUSINESS EVENTS
```

List business event services in a specific module:

```sql
SHOW BUSINESS EVENTS IN OrderModule
```

## See Also

[SHOW MODULES](show-modules.md), [DESCRIBE STRUCTURE](show-structure.md)
