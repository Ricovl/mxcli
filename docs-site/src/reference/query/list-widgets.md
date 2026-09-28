# LIST WIDGETS

## Synopsis

    LIST WIDGETS [IN <module>] [WHERE <condition>]

## Description

Lists widgets used across pages in the project. This command queries the catalog and requires `REFRESH CATALOG FULL` to have been run first. Without filters, lists all widgets. The `IN` clause restricts results to a specific module, and the `WHERE` clause enables filtering on widget properties.

This is useful for auditing widget usage, finding pages that use a particular widget type, or planning bulk widget updates.

## Parameters

*module*
: The name of the module to filter by. Only widgets on pages belonging to this module are shown.

*condition*
: A filter expression on widget properties. Supported filter columns include `WidgetType`, `PageName`, `ModuleName`, and other catalog fields.

## Examples

List all widgets (requires catalog):

```sql
REFRESH CATALOG FULL
LIST WIDGETS
```

List widgets in a specific module:

```sql
LIST WIDGETS IN Sales
```

Filter widgets by type:

```sql
LIST WIDGETS WHERE WidgetType = 'DataGrid'
```

## See Also

[LIST PAGES](list-pages.md), [DESCRIBE PAGE](describe-page.md), [DESCRIBE STRUCTURE](describe-structure.md)
