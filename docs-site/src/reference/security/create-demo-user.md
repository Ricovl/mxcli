# CREATE DEMO USER

## Synopsis

```sql
CREATE DEMO USER 'username' PASSWORD 'password'
    [ ENTITY module.Entity ]
    ( UserRole [, ...] )
```

## Description

Creates a demo user for development and testing. Demo users appear on the login screen when running the application locally, allowing quick login without manual credential entry.

Demo users require that project security has demo users enabled (`ALTER APP SECURITY DEMO USERS ON`).

The optional `ENTITY` clause specifies which entity (a specialization of `System.User`) stores the demo user. If omitted, the system auto-detects the unique `System.User` subtype in the project (typically `Administration.Account`).

## Parameters

`'username'`
:   The login name for the demo user. Enclosed in single quotes.

`PASSWORD 'password'`
:   The password for the demo user. Enclosed in single quotes.

`ENTITY module.Entity`
:   Optional. The entity that generalizes `System.User` (e.g., `Administration.Account`). If the project has exactly one `System.User` subtype, this can be omitted and it will be auto-detected.

`UserRole [, ...]`
:   One or more project-level user role names (unqualified) to assign to the demo user.

## Examples

Create a demo user with auto-detected entity:

```sql
CREATE DEMO USER 'demo_admin' PASSWORD 'Admin123!' (AppAdmin);
```

Create a demo user with an explicit entity:

```sql
CREATE DEMO USER 'demo_admin' PASSWORD 'Admin123!'
    ENTITY Administration.Account (AppAdmin);
```

Create multiple demo users for different roles:

```sql
CREATE DEMO USER 'admin' PASSWORD '1' ENTITY Administration.Account (AppAdmin);
CREATE DEMO USER 'user' PASSWORD '1' ENTITY Administration.Account (AppUser);
CREATE DEMO USER 'viewer' PASSWORD '1' ENTITY Administration.Account (AppViewer);
```

## See Also

[CREATE USER ROLE](create-user-role.md), [GRANT](grant.md)
