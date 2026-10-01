# CREATE DEMO USER

## Synopsis

```sql
CREATE [ OR MODIFY ] DEMO USER 'username' (
    Password: 'password',
    [ Entity: module.Entity, ]
    UserRoles: ( UserRole [, ...] )
)
```

## Description

Creates a demo user for development and testing. Demo users appear on the login screen when running the application locally, allowing quick login without manual credential entry.

Demo users require that project security has demo users enabled (`ALTER APP SECURITY ( EnableDemoUsers: TRUE )`).

The properties are a `( Key: value )` list, with the names Studio Pro gives them. The clause form `PASSWORD 'password' [ ENTITY module.Entity ] ( UserRole, … )` is its deprecated alias (`MDL-DEPR137`); `mxcli fmt --upgrade` rewrites it.

The optional `Entity` property specifies which entity (a specialization of `System.User`) stores the demo user. If omitted, the system auto-detects the unique `System.User` subtype in the project (typically `Administration.Account`).

## Parameters

`'username'`
:   The login name for the demo user. Enclosed in single quotes.

`Password: 'password'`
:   The password for the demo user. Enclosed in single quotes. Required.

`Entity: module.Entity`
:   Optional. The entity that generalizes `System.User` (e.g., `Administration.Account`). If the project has exactly one `System.User` subtype, this can be omitted and it will be auto-detected.

`UserRoles: ( UserRole [, ...] )`
:   The project-level user role names (unqualified) to assign to the demo user.

## Examples

Create a demo user with auto-detected entity:

```sql
CREATE DEMO USER 'demo_admin' ( Password: 'Admin123!', UserRoles: (AppAdmin) );
```

Create a demo user with an explicit entity:

```sql
CREATE DEMO USER 'demo_admin' ( Password: 'Admin123!', Entity: Administration.Account, UserRoles: (AppAdmin) );
```

Create multiple demo users for different roles:

```sql
mdl 1;
CREATE DEMO USER 'admin' ( Password: '1', Entity: Administration.Account, UserRoles: (AppAdmin) );
CREATE DEMO USER 'user' ( Password: '1', Entity: Administration.Account, UserRoles: (AppUser) );
CREATE DEMO USER 'viewer' ( Password: '1', Entity: Administration.Account, UserRoles: (AppViewer) );
```

## See Also

[CREATE USER ROLE](create-user-role.md), [GRANT](grant.md)
