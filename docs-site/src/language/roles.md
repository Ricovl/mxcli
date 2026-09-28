# Module Roles and User Roles

Mendix security uses a two-tier role system. **Module roles** define permissions within a single module. **User roles** aggregate module roles across the entire project.

## Module Roles

A module role represents a set of permissions within a module. Entity access rules, microflow access, and page access are all granted to module roles.

### CREATE MODULE ROLE

```sql
CREATE MODULE ROLE <Module>.<Role> [DESCRIPTION '<text>'];
```

Examples:

```sql
CREATE MODULE ROLE Shop.Admin DESCRIPTION 'Full administrative access';
CREATE MODULE ROLE Shop.User DESCRIPTION 'Standard customer-facing role';
CREATE MODULE ROLE Shop.Viewer;
```

### DROP MODULE ROLE

```sql
DROP MODULE ROLE Shop.Viewer;
```

### Listing Module Roles

```sql
SHOW MODULE ROLES;
SHOW MODULE ROLES IN Shop;
```

## User Roles

A user role is a project-level role assigned to end users at login. Each user role includes one or more module roles, granting the user the combined permissions of all included module roles.

### CREATE USER ROLE

```sql
CREATE USER ROLE <Name> [( ModuleRoles: (<Module>.<Role> [, ...]), Description: '<text>',
    ManageAllRoles: true|false, ManageableRoles: (<UserRole> [, ...]),
    ManageUsersWithoutRoles: true|false, CheckSecurity: true|false )];
```

The `MANAGE ALL ROLES` option allows users with this role to assign any user role to other users (typically for administrators).

Examples:

```sql
-- Administrator with management rights
CREATE USER ROLE AppAdmin ( ModuleRoles: (Shop.Admin, System.Administrator), ManageAllRoles: true );

-- Regular user
CREATE USER ROLE AppUser ( ModuleRoles: (Shop.User) );

-- Read-only viewer
CREATE USER ROLE AppViewer ( ModuleRoles: (Shop.Viewer) );
```

### ALTER USER ROLE

Add or drop module roles from an existing user role:

```sql
ALTER USER ROLE AppAdmin ADD MODULE ROLES (Reporting.Admin);
ALTER USER ROLE AppUser DROP MODULE ROLES (Shop.Viewer);
```

### DROP USER ROLE

```sql
DROP USER ROLE AppViewer;
```

### Listing User Roles

```sql
SHOW USER ROLES;
```

## Typical Setup

A common pattern is to create module roles first, then compose them into user roles:

```sql
-- 1. Module roles
CREATE MODULE ROLE Shop.Admin DESCRIPTION 'Full shop access';
CREATE MODULE ROLE Shop.User DESCRIPTION 'Standard shop access';
CREATE MODULE ROLE Reporting.Viewer DESCRIPTION 'View reports';

-- 2. User roles
CREATE USER ROLE Administrator ( ModuleRoles: (Shop.Admin, Reporting.Viewer, System.Administrator), ManageAllRoles: true );
CREATE USER ROLE Employee ( ModuleRoles: (Shop.User, Reporting.Viewer) );
CREATE USER ROLE Guest ( ModuleRoles: (Shop.User) );
```

## See Also

- [Security](./security.md) -- overview of the security model
- [Entity Access](./entity-access.md) -- granting CRUD permissions to module roles
- [Document Access](./document-access.md) -- granting microflow and page access
- [GRANT / REVOKE](./grant-revoke.md) -- the GRANT and REVOKE statements
- [Demo Users](./demo-users.md) -- creating test accounts with user roles
