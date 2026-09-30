# CREATE USER ROLE

## Synopsis

```sql
CREATE [ OR MODIFY ] USER ROLE Name [ (
    ModuleRoles: ( module.Role [, ...] ),
    Description: 'text',
    ManageAllRoles: true | false,
    ManageableRoles: ( UserRole [, ...] ),
    ManageUsersWithoutRoles: true | false,
    CheckSecurity: true | false
) ]
```

## Description

Creates a project-level user role that aggregates module roles. User roles are assigned to end users (either directly or via demo users) and determine the combined set of permissions across all modules.

A user role name must be unique at the project level. It is not qualified with a module name because user roles span multiple modules.

The properties are a `( Key: value )` list. Every property is optional, and so is the list: `CREATE USER ROLE Guest;` creates a role with no module roles. A property left out gets Mendix's default on a new role, and keeps its stored value under `CREATE OR MODIFY`. `CREATE OR MODIFY` adds the listed module roles to the ones the role already has; use `ALTER USER ROLE … DROP MODULE ROLES` to take one away.

The positional form `CREATE USER ROLE Name (module.Role, ...) [MANAGE ALL ROLES]` is deprecated (MDL-DEPR710). It still parses, and `mxcli fmt --upgrade` rewrites it.

## Parameters

`Name`
:   The name of the user role. Not module-qualified. Must be unique among all user roles in the project.

`ModuleRoles`
:   The module roles the user role includes, each a qualified name (`Module.RoleName`). The module roles must already exist.

`Description`
:   The role's description, as Studio Pro shows it.

`ManageAllRoles`
:   When `true`, users with this role can manage (assign/unassign) all user roles in the application. Typically reserved for administrator roles.

`ManageableRoles`
:   The user roles users with this role can manage, when `ManageAllRoles` is not `true`.

`ManageUsersWithoutRoles`
:   When `true`, users with this role can also manage users that have no role.

`CheckSecurity`
:   Studio Pro's "Check security" setting for the role.

## Examples

Create an administrator role with management privileges:

```sql
CREATE USER ROLE AppAdmin ( ModuleRoles: (Shop.Admin, System.Administrator), ManageAllRoles: true );
```

Create a regular user role:

```sql
CREATE USER ROLE AppUser ( ModuleRoles: (Shop.User, Notifications.Viewer) );
```

Create a role that spans multiple modules, with a description:

```sql
CREATE USER ROLE SalesManager (
    ModuleRoles: (Sales.Manager, Inventory.Viewer, Reports.User, Administration.User),
    Description: 'Runs the sales team',
    ManageableRoles: (SalesRep),
    CheckSecurity: true
);
```

## See Also

[CREATE MODULE ROLE](create-module-role.md), [GRANT](grant.md), [CREATE DEMO USER](create-demo-user.md)
