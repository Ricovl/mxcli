# LIST PAGES / LIST SNIPPETS

## Synopsis

    LIST PAGES [IN <module>]

    LIST SNIPPETS [IN <module>]

## Description

Lists pages or snippets in the project. Without the `IN` clause, lists all pages (or snippets) across all modules. With `IN <module>`, restricts the listing to the specified module.

Pages are the user interface screens of a Mendix application. Snippets are reusable page fragments that can be embedded in multiple pages via `SNIPPETCALL` widgets.

## Parameters

*module*
: The name of the module to filter by. Only pages or snippets belonging to this module are shown.

## Examples

List all pages in the project:

```sql
LIST PAGES
```

List pages in a specific module:

```sql
LIST PAGES IN Sales
```

List all snippets:

```sql
LIST SNIPPETS
```

List snippets in a specific module:

```sql
LIST SNIPPETS IN Common
```

## See Also

[DESCRIBE PAGE](describe-page.md), [LIST WIDGETS](list-widgets.md), [LIST MODULES](list-modules.md)
