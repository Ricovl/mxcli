# Home Pages and Menus

Each navigation profile has a default home page, optional role-specific home pages, and a menu tree. These determine what users see when they first open the application and how they navigate between pages.

## Home Pages

### Default Home Page

Every profile requires a default home page. This is shown to users whose role has no specific home page assignment:

```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web;
```

### Role-Specific Home Pages

Use `HOME PAGE ... FOR` to direct users to different pages based on their **user role**:

```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  HOME PAGE MyModule.AdminDashboard FOR Administrator
  HOME PAGE MyModule.ManagerDashboard FOR Manager;
```

> **Write the role as a bare name.** `FOR` takes a *user role*, which is
> project-level and has no module part — `FOR Administrator`, not
> `FOR MyModule.Administrator`. A *module role* is a different thing that
> happens to share the name: a blank app has a user role `Administrator` and
> module roles called `Administrator` in three modules, so the wrong one looks
> right. A module-qualified name here produces a project Mendix **cannot load**
> (`StorageLoadException: … is not a valid UserRoleIdentifier`), which is worse
> than a build error because it happens before checking runs. `mxcli check
> --references` refuses it.

When a user logs in, the runtime checks their roles and redirects to the most specific matching home page. If no role-specific page matches, the default home page is used.

### Login Page

The login page is shown to unauthenticated users:

```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  LOGIN PAGE Administration.Login;
```

### Not Found Page

An optional custom 404 page:

```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE MyModule.Home_Web
  NOT FOUND PAGE MyModule.Custom404;
```

## Menus

The `{ }` block after the profile's clauses defines the navigation menu as a tree of items and submenus.

### Menu Items

A menu item links a label to a page, a microflow, or sign-out, in the words a page action uses:

```sql
MENU ITEM '<label>' ( OnClick: SHOW PAGE <Module>.<Page> )
MENU ITEM '<label>' ( OnClick: CALL MICROFLOW <Module>.<Microflow>, Icon: <Module>.<IconCollection>.<Icon> )
MENU ITEM '<label>' ( OnClick: SIGN OUT )
```

### Submenus

Nest items inside a `MENU '<label>' { ... }` block:

```sql
MENU '<label>' [( Icon: <icon> )] {
  <menu-items>
}
```

### Complete Example

```sql
CREATE OR MODIFY NAVIGATION Responsive
  HOME PAGE Shop.Home
  LOGIN PAGE Administration.Login
  {
    MENU ITEM 'Home' ( OnClick: SHOW PAGE Shop.Home )
    MENU ITEM 'Products' ( OnClick: SHOW PAGE Shop.Product_Overview )
    MENU ITEM 'Orders' ( OnClick: SHOW PAGE Shop.Order_Overview )
    MENU 'Administration' {
      MENU ITEM 'Users' ( OnClick: SHOW PAGE Administration.Account_Overview )
      MENU ITEM 'Roles' ( OnClick: SHOW PAGE Administration.Role_Overview )
      MENU 'System' {
        MENU ITEM 'Logs' ( OnClick: SHOW PAGE Administration.Log_Overview )
        MENU ITEM 'Settings' ( OnClick: SHOW PAGE Shop.Settings )
      }
    }
  };
```

### Inspecting Menus

```sql
-- View menu tree
LIST NAVIGATION MENU;
LIST NAVIGATION MENU Responsive;

-- View home page assignments
LIST NAVIGATION HOMES;
```

## See Also

- [Navigation and Settings](./navigation.md) -- overview of navigation concepts
- [Navigation Profiles](./navigation-profiles.md) -- profile types and CREATE OR REPLACE syntax
- [Project Settings](./project-settings.md) -- runtime and configuration settings
