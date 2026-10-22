# ALTER PAGE / ALTER SNIPPET

## Synopsis

```sql
ALTER PAGE module.Name {
    operations
}

ALTER SNIPPET module.Name {
    operations
}
```

`ALTER LAYOUT module.Name { … }` takes the same operations. This is the
generic ALTER — `set` / `insert` / `replace` / `drop` against a target — which
every document type shares; a page's targets are widget names, DataGrid 2
columns `grid column(Attr)` / `grid column('Caption')`, navigation list items
`nav item(widget)` / `nav item('Text')` / `nav item@n`, or a layout region
`container.top`.

The older spellings still run and warn with a deprecation code:

| Old spelling | Write instead | Code |
|---|---|---|
| `SET Caption = 'x' ON w`, `SET (A = 1, B = 2) ON w` | `SET (Caption: 'x') ON w`, `SET (A: 1, B: 2) ON w` | MDL-DEPR101 |
| `SET Caption: 'x' ON w` (no parentheses) | `SET (Caption: 'x') ON w` | MDL-DEPR102 |
| `DROP WIDGET a, b` | `DROP a, b` | MDL-DEPR103 |

Where each operation is one of:

```sql
-- Set a property on a widget
SET (property: value) ON widgetName;
SET ( property1: value1, property2: value2 ) ON widgetName;

-- Set a page-level property (no ON clause)
SET (Title: 'New Title');

-- Set a pluggable widget property (quoted name)
SET ('propertyName': value) ON widgetName;

-- Insert widgets before or after a target
INSERT BEFORE widgetName { widget_definitions };
INSERT AFTER widgetName { widget_definitions };

-- Insert widgets as the last children of a container
INSERT INTO containerName { widget_definitions };

-- Remove widgets
DROP widgetName1, widgetName2;

-- Replace a widget with new widgets
REPLACE widgetName WITH { widget_definitions };

-- Add a page variable
ADD Variables $name : type = <expression>;

-- Drop a page variable
DROP Variables $name;

-- Change page layout (auto-map placeholders by name)
SET Layout = module.LayoutName;

-- Change layout with explicit placeholder mapping
SET Layout = module.LayoutName MAP (OldPlaceholder AS NewPlaceholder);
```

## Description

Modifies an existing page or snippet in-place without requiring a full `CREATE OR REPLACE`. This is useful when you need to make targeted changes to a page while preserving the rest of its widget tree, including any unsupported or third-party widget types that cannot be round-tripped through MDL.

ALTER PAGE works directly on the raw BSON widget tree, so it preserves widgets and properties that MDL does not explicitly support.

### SET

Sets one or more properties on a named widget. Widget names are assigned during `CREATE PAGE` and can be discovered with `DESCRIBE PAGE`.

Supported standard properties: `Caption`, `Label`, `ButtonStyle`, `Class`, `Style`, `Editable`, `Visible`, `Name`.

For page-level properties (like `Title`), omit the `ON` clause.

For pluggable widget properties, use quoted property names (e.g., `'showLabel'`).

### INSERT BEFORE / INSERT AFTER

Inserts new widgets immediately before or after a named widget within its parent container. The new widgets use the same syntax as in `CREATE PAGE`.

### INSERT INTO

Appends new widgets as the **last children** of a named container. This is the only way to fill an **empty** container, and the natural way to add a widget to a container or data view without needing a sibling to anchor to. Widgets inserted into a data view take that data view's entity as their context.

Supported on simple containers (container/DivContainer, data view, group box, tab page, scroll-container region). A layout grid (rows/columns) has no single child list — insert relative to a widget inside the target column instead. A tab container's children are tab pages; see below.

A pluggable widget's child-widget slots have no name of their own, so they are addressed by the widget and the slot keyword `describe` prints: `insert into dataGrid1.controlbar { actionbutton … }` adds to a Data grid 2's control bar (even after its last button was dropped), `replace dataGrid1.controlbar with { … }` swaps its content, and `drop dataGrid1.controlbar` empties it. The widget's own property key (`dataGrid1.filtersPlaceholder`) works too. `insert into dataGrid1` on the widget itself is refused with the slot addresses it has.

### Adding Tab Pages

A tab container holds tab pages, and only tab pages. `INSERT INTO` the tab
container appends one; `INSERT BEFORE` / `INSERT AFTER` a tab page places it next
to that sibling. The container keeps its default tab.

```sql
mdl 1;
alter page ModT.TabsPage {
  insert after tpOne { tabpage tpTwo (Caption: 'Two') { dynamictext txtTwo (Content: 'two') } }
};
alter page ModT.TabsPage {
  insert into tabsMain { tabpage tpThree (Caption: 'Three') { dynamictext txtThree (Content: 'three') } }
};
```

A tab page cannot go next to an ordinary widget or inside another tab page, and
one `INSERT` cannot mix tab pages with widgets. Dropping, replacing or reordering
tab pages still needs `CREATE OR MODIFY PAGE`.

A navigation list holds items, and only items. `INSERT INTO` the list appends
one and leaves the existing items as they are. The widgets inside an item are
targets by their own names.

```sql
mdl 1;
alter snippet Shop.Menu {
  insert into navList1 { item (Action: show page Shop.Home) { dynamictext t2 (Content: 'Second') } }
};
alter snippet Shop.Menu {
  insert after t2 { dynamictext t3 (Content: 'Third') }
};
```

### Navigation List Item Operations

A navigation list item has no stored name either, so — like a DataGrid 2
column — it is addressed by what it holds or where it is:

| Address | Means |
|---------|-------|
| `nav item(t1)` | the item holding the widget named `t1` |
| `nav item('Home')` | the item showing the text `Home` (any language) |
| `nav item@2` | the second item |
| `nav item('Home')@2` | the second of several items showing `Home` |

An address that matches several items is refused unless `@n` picks one, and a
miss lists every item's addresses. Prefer the content forms: a position moves
when an item is inserted before it.

```sql
mdl 1;
alter snippet Shop.Menu {
  insert before navList1 item@1 { item (Action: show page Shop.Orders) { dynamictext t0 (Content: 'Orders') } }
  insert after navList1 item(t1) { item (Action: show page Shop.Home) { dynamictext t5 (Content: 'Home') } }
  set (Action: show page Shop.Orders) on navList1 item('Home')@2;
  insert into navList1 item('Orders') { dynamictext t0b (Content: 'All orders') }
  replace navList1 item(t2) with { item (Action: show page Shop.Home) { dynamictext t2 (Content: 'Second') } }
  drop navList1 item@3;
};
```

Only items go next to an item or in its place: `INSERT BEFORE` / `AFTER` an item
and `REPLACE` of one take `item` blocks, and widgets go `INTO` an item.

### Pluggable Widget Item Operations

A pluggable widget's object-list item — an Accordion group, a Pop-up menu
item — stores no name either (a name written on `group g1` is not kept), and is
addressed the same way, on the widget that owns the list:

| Address | Means |
|---------|-------|
| `acc1 item@2` | the second group — `INSERT INTO` fills its body |
| `acc1 item('One')` | the group whose text (`headerText`) is `One` |
| `acc1 item(btnIn)` | the group holding the widget `btnIn` |
| `acc1 item@1.headercontent` | another slot of the group, by its keyword |

The body is the slot `describe` prints as the item's children (`content`, or
the item's first widget slot); any other slot is named with `.slot`, spelled as
`describe` prints its block.

```sql
mdl 1;
alter page Shop.Faq {
  insert into acc1 item@2 { dynamictext txtAnswer (Content: 'Yes.') };
  set (headerRenderMode: 'custom') on acc1 item@1;
  insert into acc1 item@1.headercontent { dynamictext txtHead (Content: 'Shipping') };
  replace acc1 item('Returns').content with { dynamictext txtRet (Content: '30 days') };
  drop acc1 item@3;
};
```

- A slot the widget's configuration hides is refused, naming the property that
  shows it: an Accordion hides a group's `headercontent` while
  `headerRenderMode` is `'text'` (the default). Widgets written there are not
  rendered and fail the build with CE0463. Set the property first — in the same
  statement is fine, as above.
- A SET that switches a group's mode keeps the texts it hides or shows as
  CREATE stores them. Switching back to `'text'` needs the text in the same
  statement — `set (headerRenderMode: 'text', headerText: 'Shipping') on acc1 item@1`
  — since the stored schema has no default to fill in.
- `INSERT BEFORE` / `AFTER` an item (adding a new group) and `REPLACE` of a
  whole item are not supported; use `drop` and `create or modify page`.

### DROP

Removes one or more widgets by name. The widget and all its children are removed from the tree.

### REPLACE

Replaces a widget (and its entire subtree) with one or more new widgets.

### DataGrid Column Operations

A DataGrid 2 column has no stored name. It is addressed by the attribute or the
caption `DESCRIBE PAGE` prints in it: `grid column(Attr)`, `grid column('Caption')`.
An address two columns share is refused unless `@n` picks one.

```sql
SET (Caption: 'Product SKU') ON dgProducts column(Code)
DROP dgProducts column('Old column')
INSERT AFTER dgProducts column(Price) { COLUMN (Attribute: Margin) }
REPLACE dgProducts column(Description) WITH { COLUMN (Attribute: Notes) }
DROP dgProducts column(Name)@2
```

The older dotted form `gridName.columnName` still works, matching a name mxcli
derives from the attribute or caption.

### SET Layout

Changes the page's layout without rebuilding the widget tree. Placeholder names are auto-mapped by default. If the new layout has different placeholder names, use `MAP` to specify the mapping.

A repoint that would leave the page bound to a placeholder the target layout does not declare is **refused**, and the error names `MAP` as the remedy. The check runs after `MAP` is applied, so mapping onto an existing placeholder is always accepted. To move many pages at once, see [ALTER LAYOUT](alter-layout.md), which documents the bulk `ALTER PAGES … SET LAYOUT` form.

Not supported for snippets (snippets don't have layouts).

### ADD Variables / DROP Variables

Adds or removes page-level variables. Page variables are typed values with default expressions, available for conditional visibility and dynamic behavior.

## Parameters

`module.Name`
:   The qualified name of the page or snippet to modify. Must already exist.

`widgetName`
:   The name of a widget within the page. Use `DESCRIBE PAGE` to list widget names.

`property`
:   A widget property name. Standard properties are unquoted. Pluggable widget properties use single quotes.

`value`
:   The new property value. Strings are quoted. Booleans and enumerations are unquoted.

## Examples

Change button caption and style:

```sql
ALTER PAGE Sales.Order_Edit {
    SET (Caption: 'Save & Close', ButtonStyle: Success) ON btnSave;
};
```

Remove an unused widget and add a new field:

```sql
ALTER PAGE Sales.Order_Edit {
    DROP txtUnused;
    INSERT AFTER txtEmail {
        TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
    }
};
```

Replace a widget with multiple new widgets:

```sql
ALTER PAGE Sales.Order_Edit {
    REPLACE txtOldField WITH {
        TEXTBOX txtFirstName (Label: 'First Name', Attribute: FirstName)
        TEXTBOX txtLastName (Label: 'Last Name', Attribute: LastName)
    }
};
```

Set a page-level property:

```sql
ALTER PAGE Sales.Order_Edit {
    SET (Title: 'Edit Order Details');
};
```

Set a pluggable widget property:

```sql
ALTER PAGE Sales.Order_Edit {
    SET ('showLabel': false) ON cbStatus;
};
```

Add and drop page variables:

```sql
mdl 1;
ALTER PAGE Sales.Order_Edit {
    ADD Variables $showAdvanced : Boolean = false;
};

ALTER PAGE Sales.Order_Edit {
    DROP Variables $showAdvanced;
};
```

Change page layout (preserves all widgets):

```sql
ALTER PAGE Sales.Order_Edit {
    SET Layout = Atlas_Core.Atlas_Default;
};
```

Change layout with explicit placeholder mapping:

```sql
ALTER PAGE Sales.Order_Edit {
    SET Layout = Atlas_Core.Atlas_SideBar MAP (Main AS Content, Extra AS Sidebar);
};
```

Modify a snippet:

```sql
ALTER SNIPPET MyModule.NavMenu {
    SET (Caption: 'Dashboard') ON btnHome;
    INSERT AFTER btnHome {
        ACTIONBUTTON btnReports (Caption: 'Reports', Action: PAGE MyModule.Reports)
    }
};
```

Combined operations in a single ALTER:

```sql
ALTER PAGE MyModule.Customer_Edit {
    SET (Title: 'Customer Details');
    SET (Caption: 'Update', ButtonStyle: Primary) ON btnSave;
    DROP txtObsolete;
    INSERT BEFORE txtEmail {
        TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
    }
    INSERT AFTER txtEmail {
        COMBOBOX cbCategory (Label: 'Category', Attribute: Category)
    }
};
```

## See Also

[CREATE PAGE](create-page.md), [CREATE SNIPPET](create-snippet.md), [DESCRIBE PAGE](/reference/query/describe-page.md), [DROP PAGE](drop-page.md)
