# CREATE MODULE ROLE

## Synopsis

```sql
CREATE MODULE ROLE module.RoleName [ DESCRIPTION 'text' ]
```

## Description

Creates a new module role within a module. Module roles are the foundation of Mendix security -- they are referenced by entity access rules, microflow/page access grants, and aggregated into project-level user roles.

Each module role belongs to exactly one module. The role name must be unique within the module. By convention, common role names include `User`, `Admin`, `Viewer`, and `Manager`.

If the module does not exist, the statement returns an error.

## Parameters

`module.RoleName`
:   Qualified name consisting of the module name and the new role name, separated by a dot. The module must already exist.

`DESCRIPTION 'text'`
:   Optional human-readable description of the role's purpose.

## Examples

Create a basic module role:

```sql
CREATE MODULE ROLE Shop.Admin;
```

Create a role with a description:

```sql
CREATE MODULE ROLE Shop.Admin DESCRIPTION 'Full administrative access to shop module';
```

Create multiple roles for a module:

```sql
mdl 1;
CREATE MODULE ROLE HR.Manager DESCRIPTION 'Can manage employees';
CREATE MODULE ROLE HR.Employee DESCRIPTION 'Self-service access';
CREATE MODULE ROLE HR.Viewer DESCRIPTION 'Read-only access';
```

## See Also

[CREATE USER ROLE](create-user-role.md), [GRANT](grant.md), [REVOKE](revoke.md)
