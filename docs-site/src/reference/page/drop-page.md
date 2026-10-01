# DROP PAGE / SNIPPET

## Synopsis

```sql
mdl 1;
DROP PAGE module.Name;

DROP SNIPPET module.Name;
```

## Description

Removes a page or snippet from the project. The page or snippet must exist; otherwise an error is raised.

Dropping a page or snippet does not automatically update references to it from microflows (e.g., `SHOW PAGE`), navigation menus, other pages (`SNIPPETCALL`), or security rules. Use `LIST IMPACT OF module.Name` before dropping to identify all references that will break.

## Parameters

`module.Name`
:   The qualified name of the page or snippet to drop (`Module.PageName` or `Module.SnippetName`).

## Examples

Drop a page:

```sql
DROP PAGE Sales.Order_Edit;
```

Drop a snippet:

```sql
DROP SNIPPET MyModule.CustomerHeader;
```

Check impact before dropping:

```sql
mdl 1;
-- See what references this page
LIST IMPACT OF Sales.Order_Edit;

-- Then drop if safe
DROP PAGE Sales.Order_Edit;
```

## See Also

[CREATE PAGE](create-page.md), [CREATE SNIPPET](create-snippet.md), [ALTER PAGE](alter-page.md), [LIST IMPACT](/reference/catalog/list-references-impact.md)
