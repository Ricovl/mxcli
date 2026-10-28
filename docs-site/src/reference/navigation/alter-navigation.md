# ALTER NAVIGATION

## Synopsis

```text
ALTER NAVIGATION profile {
    item_operation | page_clause_operation
    [ ... ]
};

-- item_operation
INSERT { BEFORE | AFTER | INTO } item { menu_items }
INSERT { menu_items }
REPLACE item WITH { menu_items }
MOVE item { BEFORE | AFTER | INTO } item
DROP item [, item ...]
SET ( [ Caption: 'text' ] [, OnClick: action | NOTHING ] [, Icon: icon | NONE ] ) ON item

-- page_clause_operation
SET HOME { PAGE | MICROFLOW } module.Name [ FOR UserRole ]
DROP HOME PAGE FOR UserRole
SET LOGIN PAGE module.Page
DROP LOGIN PAGE
SET NOT FOUND PAGE module.Page
DROP NOT FOUND PAGE

-- item
'caption' [ @n ] [ > 'caption' [ @n ] ... ]
```

## Description

Edits one web navigation profile in place: its menu items at any depth, and its
home, login and not-found pages. Where
[CREATE OR MODIFY NAVIGATION](create-navigation.md) replaces the whole menu with
the `{ }` block it is given, `ALTER NAVIGATION` changes only what it names. Adding
one item does not mean restating the menu, an item someone added in Studio Pro
meanwhile is kept, and nothing the statement does not name is re-validated.

The operations apply in order to the stored profile, each one against the menu as
the operations before it left it, so an item inserted by one operation can be the
target of the next. The statement is written once: if any operation is refused,
nothing is written.

The same item operations apply to a menu document with
[ALTER MENU](menu.md).

### Addressing an item

A menu item has no name, so it is addressed by the caption `DESCRIBE NAVIGATION`
prints for it, written as a path from outer to inner:

| Address | Means |
|---|---|
| `'Users'` | the item captioned `Users`, anywhere in the menu |
| `'Admin' > 'Users'` | `Users` directly under `Admin` |
| `'Settings' > 'Admin' > 'Users'` | the same, one level further out |
| `'Users'@2` | the 2nd item captioned `Users`, in describe order |
| `'Admin'@2 > 'Users'` | `Users` under the 2nd item captioned `Admin` |
| `''@2` | the 2nd item with an empty caption (an icon-only item) |

- Captions compare exactly, case included, with the caption `DESCRIBE` prints —
  the project's default language, as for a page's texts.
- A path matches as a **suffix**: the first segment matches at any depth, and
  each `>` is one **direct** parent/child step.
- `@n` belongs to the segment it follows and picks the n-th of that segment's
  matches in describe order.
- An address that matches nothing, or several items, is refused. The error for
  an ambiguous address lists the shortest address of every candidate:

  ```
  menu item 'Users' is ambiguous: it matches 2 items — write one of these:
    'Admin' > 'Users'
    'Reports' > 'Users'
  ```

- There are no positional addresses. [`LIST NAVIGATION MENU`](list-navigation.md)
  prints the address of every item.

### Item operations

`INSERT BEFORE | AFTER item { … }`
:   Places the items as siblings of `item`.

`INSERT INTO item { … }`
:   Appends them as `item`'s last sub-items. `item` must have no action
    (Mendix CE0548); use `SET ( OnClick: NOTHING ) ON item` first.

`INSERT { … }`
:   Appends them to the top level. This is also how to add to an empty menu.

`REPLACE item WITH { … }`
:   Puts the items where `item` was; its sub-items go with it unless the new
    items state them again. A replacement keeps the other-language translations
    of a caption it does not change.

`MOVE item BEFORE | AFTER | INTO item`
:   Moves the stored item itself — its sub-items, every caption translation and
    any action MDL cannot spell go with it. Moving an item into its own
    sub-items is refused.

`DROP item, …`
:   Removes each item and its sub-items.

`SET ( … ) ON item`
:   Changes only the keys written. `Caption:` writes the project's default
    language and keeps the other translations. `OnClick:` takes the action
    expression a menu item takes in `CREATE NAVIGATION`; `NOTHING` stores
    Studio Pro's "Do nothing", the only action an item with sub-items may have.
    `Icon:` takes an icon-collection name, `GLYPH n` or `IMAGE module.Images.name`,
    or `NONE` for no icon.

The `{ menu_items }` of `INSERT` and `REPLACE` are written exactly as in
`CREATE NAVIGATION`, so a line `DESCRIBE NAVIGATION` printed can be pasted in.

### Page clause operations

Each is the `CREATE NAVIGATION` clause after `SET` or `DROP`.

| Operation | Effect |
|---|---|
| `SET HOME PAGE M.P` / `SET HOME MICROFLOW M.F` | the default home |
| `SET HOME PAGE M.P FOR Role` | that user role's home; other roles keep theirs |
| `DROP HOME PAGE FOR Role` | removes that role's home, page or microflow |
| `SET LOGIN PAGE M.P` / `DROP LOGIN PAGE` | sets or clears the login page |
| `SET NOT FOUND PAGE M.P` / `DROP NOT FOUND PAGE` | sets or clears the fallback page |

There is no `DROP HOME PAGE` without `FOR`: a web profile needs a default home
page (Mendix CE0527), so `SET HOME PAGE` changes it instead. `FOR` takes a bare
user role, as in `CREATE NAVIGATION`.

### What is refused

- an address that matches nothing or several items, or an `@n` beyond the matches;
- `INSERT INTO` or `MOVE … INTO` an item with an action, and an `OnClick:` other
  than `NOTHING` on an item with sub-items (CE0548);
- `DROP HOME PAGE` without `FOR` (CE0527), and `DROP HOME PAGE FOR` a role that
  has none;
- a native profile — its bottom bar is not written by mxcli; set its home pages
  with `CREATE OR MODIFY NAVIGATION`;
- a profile the project does not have — `CREATE OR MODIFY NAVIGATION` adds one.

`mxcli check script.mdl -p app.mpr --references` resolves every address against
the stored menu with the same code `exec` runs and reports the same refusals.

## Parameters

`profile`
:   A web navigation profile the project has: `Responsive`, `Phone`, `Tablet` or
    one of their `Offline` kinds.

## Examples

Given the menu `DESCRIBE NAVIGATION Responsive` prints:

```sql
CREATE OR MODIFY NAVIGATION Responsive
    HOME PAGE Shop.Home_Web
    HOME PAGE Shop.AdminHome FOR Administrator
    LOGIN PAGE Administration.Login
    NOT FOUND PAGE Shop.NotFound
{
    MENU ITEM 'Home' ( OnClick: SHOW PAGE Shop.Home_Web )
    MENU 'Admin' {
        MENU ITEM 'Users' ( OnClick: SHOW PAGE Administration.Account_Overview )
        MENU ITEM 'Old import' ( OnClick: CALL MICROFLOW Shop.ACT_LegacyImport )
    }
    MENU 'Reports' {
        MENU ITEM 'Users' ( OnClick: SHOW PAGE Shop.Report_Users )
    }
    MENU ITEM 'Help' ( OnClick: OPEN LINK 'https://example.com/help' )
    MENU ITEM 'Log out' ( OnClick: SIGN OUT )
};
```

one statement makes several unrelated edits and touches nothing else:

```sql
ALTER NAVIGATION Responsive {
    INSERT AFTER 'Home' {
        MENU ITEM 'Orders' ( OnClick: SHOW PAGE Shop.Order_Overview )
    }
    INSERT INTO 'Admin' {
        MENU ITEM 'Audit log' ( OnClick: SHOW PAGE Shop.AuditLog_Overview )
    }
    SET ( Caption: 'User accounts' ) ON 'Admin' > 'Users';
    REPLACE 'Help' WITH {
        MENU 'Help' {
            MENU ITEM 'Documentation' ( OnClick: OPEN LINK 'https://example.com/docs' )
            MENU ITEM 'Contact' ( OnClick: SHOW PAGE Shop.Contact )
        }
    }
    MOVE 'Log out' AFTER 'Help';
    DROP 'Old import';
    SET LOGIN PAGE Shop.CustomLogin;
    DROP NOT FOUND PAGE;
};
```

A bare `'Users'` in that statement would be refused: it matches two items.

## See Also

[CREATE NAVIGATION](create-navigation.md), [LIST NAVIGATION](list-navigation.md),
[CREATE MENU](menu.md)
