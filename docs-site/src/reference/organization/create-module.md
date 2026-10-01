# CREATE MODULE

## Synopsis

    CREATE MODULE module_name

## Description

Creates a new module in the Mendix project. Modules are the top-level organizational unit for all project documents including entities, microflows, pages, and enumerations. The module is created with default settings and an empty domain model.

## Parameters

**module_name**
: The name of the module to create. Must be a valid identifier (letters, digits, underscores; cannot start with a digit). If the name conflicts with a reserved keyword, enclose it in double quotes.

## Examples

### Create a module

```sql
CREATE MODULE OrderManagement;
```

### Create multiple modules for a layered architecture

```sql
mdl 1;
CREATE MODULE Sales;
CREATE MODULE Inventory;
CREATE MODULE Reporting;
```

## See Also

[CREATE FOLDER](create-folder.md), [MOVE](move.md), [DROP MODULE](../domain-model/drop-entity.md)
