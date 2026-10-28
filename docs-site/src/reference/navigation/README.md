# Navigation Statements

Statements for configuring application navigation profiles, home pages, login pages, and menu structures.

Mendix applications can have multiple navigation profiles (Responsive, Tablet, Phone, NativePhone) that each define a home page, optional login page, optional 404 page, and a hierarchical menu tree. Navigation profiles control which page the user sees first and how they navigate through the application.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE NAVIGATION](create-navigation.md) | Create a navigation profile, or write the home pages, login page, menu and sync clauses it states |
| [ALTER NAVIGATION](alter-navigation.md) | Edit one profile's menu items and page clauses in place: insert, replace, move, drop, set |
| [LIST NAVIGATION](list-navigation.md) | Display navigation profiles, menus, and home page assignments |
| [CREATE MENU](menu.md) | Create, alter, describe and drop standalone menu documents (`Menus$MenuDocument`) |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Describe navigation | `DESCRIBE NAVIGATION [profile]` |
