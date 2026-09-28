# DESCRIBE PAGE / DESCRIBE SNIPPET

## Synopsis

    DESCRIBE PAGE <qualified_name>

    DESCRIBE SNIPPET <qualified_name>

## Description

Shows the complete MDL source for a page or snippet, including page properties (title, layout, parameters), the full widget tree with all property values, and nested widget structures. The output is round-trippable MDL that can be used directly in `CREATE PAGE` or `CREATE SNIPPET` statements.

This is particularly useful before using `ALTER PAGE` or `ALTER SNIPPET`, as it reveals the widget names needed for targeted modifications.

## Parameters

*qualified_name*
: A `Module.PageName` or `Module.SnippetName` reference identifying the page or snippet to describe. Both the module and document name are required.

## Examples

Describe a page:

```sql
DESCRIBE PAGE Sales.Customer_Edit
```

Example output:

```sql
CREATE PAGE Sales.Customer_Edit
(
  Params: ( $Customer: Sales.Customer ),
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    FOOTER footer1 {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
}
```

Describe a snippet:

```sql
DESCRIBE SNIPPET Common.NavigationMenu
```

## See Also

[SHOW PAGES](show-pages.md), [SHOW WIDGETS](show-widgets.md), [DESCRIBE MICROFLOW](describe-microflow.md)
