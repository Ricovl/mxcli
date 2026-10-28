# CREATE MENU

## Synopsis

```sql
CREATE [ OR MODIFY ] MENU module.name [ FOLDER 'folder_path' ] { menu_item [ menu_item ... ] }
ALTER MENU module.name { item_operation [ item_operation ... ] }
DESCRIBE MENU module.name
DROP MENU module.name
```

Where each `menu_item` is one of:

```sql
MENU ITEM 'caption' [ ( [ OnClick: action ] [, Icon: icon ] ) ]
MENU 'caption' [ ( Icon: icon ) ] { nested_items }
```

and each `item_operation` is one of the item operations of
[ALTER NAVIGATION](alter-navigation.md#item-operations) — `INSERT`, `REPLACE`,
`MOVE`, `DROP`, `SET ( … ) ON` — with items addressed by caption path
(`'Group' > 'Item'`).

## Description

Manages standalone **menu documents** (`Menus$MenuDocument`) — reusable menus that a
menu widget on a page points at. Atlas_Core ships two of them, `Phone_Menu` and
`Tablet_Menu`.

A menu document is **not** the menu inside a navigation profile, although the two
are easy to confuse: both are built from the same menu items, which is why the item
syntax here is identical to the `{ ... }` menu block of
[CREATE NAVIGATION](create-navigation.md). They differ in where they live and how they
are read:

| | Profile menu | Menu document |
|---|---|---|
| Lives in | a navigation profile | its own document |
| Used by | the app's navigation | a menu widget you place on a page |
| Read with | `LIST NAVIGATION MENU` | `DESCRIBE MENU module.name` |
| Written with | `CREATE OR MODIFY NAVIGATION` | `CREATE OR MODIFY MENU` |
| One item at a time | `ALTER NAVIGATION` | `ALTER MENU` |

`OR MODIFY` replaces the item list **wholesale**, exactly as the `{ }` block of
`CREATE OR MODIFY NAVIGATION` does: the list you give is the document's complete
contents, so an omitted item is a removed item. The document's identity, container
and export level are preserved, so menu widgets pointing at it keep working.

`ALTER MENU` edits the stored items instead: it inserts, replaces, moves, drops or
changes the items it names and keeps every other item exactly as stored. It takes
the item operations of [ALTER NAVIGATION](alter-navigation.md) — no page clauses,
since a menu document has none:

```sql
ALTER MENU MyModule.Main_Menu {
    INSERT AFTER 'Home' { MENU ITEM 'Orders' ( OnClick: SHOW PAGE MyModule.Order_Overview ) }
    MOVE 'Run' INTO 'Admin';
    SET ( Caption: 'Accounts and roles' ) ON 'Admin' > 'Accounts';
    DROP 'Plain';
};
```

`DESCRIBE MENU` emits a re-executable `CREATE OR MODIFY MENU` statement, so
describe → edit → exec is the normal editing loop, and describe → exec → describe is
a fixed point.

## Parameters

`module.name`
:   Qualified name of the menu document.

`FOLDER 'folder_path'`
:   Optional. Places the document in the named module folder, creating missing folders in the path. On `CREATE OR MODIFY` this **moves** an existing document; omitting the clause leaves placement alone rather than returning the document to the module root. See [MOVE](../organization/move.md).

`'caption'`
:   The item's label, in single quotes.

`OnClick:`
:   Optional action run when the item is clicked, in the words a page action uses:
    `show page M.P`, `call microflow M.F`, `call nanoflow M.N`,
    `open link 'https://…'`, `create object M.E [then show page M.P]`, `sign out`
    or `nothing` — the action expression a button's `Action:` takes, with its
    `with ( … )` settings (progress bar and message, confirmation, asynchronous,
    form validations, disabled during action). Save, delete, close page and
    complete task are refused: a menu item has no object to act on. An item
    without one is inert (stored as `Forms$NoAction`), which is normal for an
    item that only groups sub-items. A page title override or a link type other
    than Web has no spelling; describe flags it, and a rewrite keeps it while the
    item's caption and action are unchanged.

`Icon:`
:   Optional icon, given as a qualified name into an icon collection — not a string.
    Hyphenated segments are double-quoted: `Atlas_Core.Atlas."layout-2"`.
    `glyph <code>` and `image Module.Images.name` write the other two icon kinds.

The items are children, in `{ }` with no separator between them (R2). The old
spelling — items in `( )`, each ending in `;`, with `PAGE`/`MICROFLOW`/`ICON`
clauses after the caption — still parses and warns (MDL-DEPR121, MDL-DEPR122);
`mxcli fmt --upgrade` rewrites it.

## Examples

Create a menu with a nested sub-menu:

```sql
CREATE MENU MyModule.Main_Menu {
  menu item 'Home' ( OnClick: show page MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )
  menu item 'Run' ( OnClick: call microflow MyModule.DoThing )
  menu 'Admin' {
    menu item 'Accounts' ( OnClick: show page Administration.Account_Overview )
  }
  menu item 'Plain'
};
```

Replace its contents (the two omitted items are removed):

```sql
CREATE OR MODIFY MENU MyModule.Main_Menu {
  menu item 'Home' ( OnClick: show page MyModule.Home_Web, Icon: Atlas_Core.Atlas.home )
  menu item 'Run' ( OnClick: call microflow MyModule.DoThing )
};
```

Read one back — including Atlas's own:

```sql
DESCRIBE MENU Atlas_Core.Phone_Menu;
```

The type is auto-detected too, so the bare form works:

```sql
DESCRIBE Atlas_Core.Phone_Menu;
```

From the command line:

```bash
mxcli describe menu Atlas_Core.Phone_Menu -p app.mpr
```

## Notes

**A menu item cannot open a page that takes a required parameter.** There is nowhere
to supply the argument, and Mendix rejects the model with **CE1571** ("No argument
has been selected for parameter …") reported against `Menu item`. Point the item at a
parameterless page, or call a microflow that opens the page with its argument.

**All three icon kinds round-trip.** `Icon: Module.Collection.name` writes an
icon-collection icon, `Icon: GLYPH n` a glyph and `Icon: IMAGE Module.Images.name`
an image icon, and `DESCRIBE` prints each the same way. Only an icon it cannot
rebuild — a glyph stored without a code, or an icon type mxcli does not know — is
reported on its own comment line rather than dropped silently:

```
-- icon a numeric glyph code (Forms$GlyphIcon) is not reproducible by CREATE MENU;
-- set it in Studio Pro
```

## See Also

- [CREATE NAVIGATION](create-navigation.md) and [ALTER NAVIGATION](alter-navigation.md) — the menu inside a navigation profile
- [LIST NAVIGATION](list-navigation.md) — read profile menus and home pages
