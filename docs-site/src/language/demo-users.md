# Demo Users

Demo users are test accounts created for development and testing. They appear on the login screen when demo users are enabled, allowing quick access without manual user setup.

## CREATE DEMO USER

```sql
CREATE DEMO USER '<username>' (
  Password: '<password>',
  [Entity: <Module>.<Entity>,]
  UserRoles: (<UserRole> [, ...])
);
```

| Parameter | Description |
|-----------|-------------|
| `<username>` | Login name for the demo user |
| `Password` | Password (visible in development only) |
| `Entity` | Optional. The entity that generalizes `System.User` (e.g., `Administration.Account`). If omitted, the system auto-detects the unique `System.User` subtype. |
| `UserRoles` | The project-level user roles to assign |

The keys are Studio Pro's property names. The clause form `PASSWORD '<password>' [ENTITY <Module>.<Entity>] (<UserRole>, …)` is the deprecated alias of the list (`MDL-DEPR137`); `mxcli fmt --upgrade` rewrites it.

### Examples

```sql
-- Basic demo user
CREATE DEMO USER 'demo_admin' ( Password: 'Admin123!', UserRoles: (Administrator) );

-- With explicit entity
CREATE DEMO USER 'demo_admin' ( Password: 'Admin123!', Entity: Administration.Account, UserRoles: (Administrator) );

-- Multiple roles
CREATE DEMO USER 'demo_manager' ( Password: 'Manager1!', UserRoles: (Manager, Reporting) );

-- Standard user
CREATE DEMO USER 'demo_user' ( Password: 'User1234!', UserRoles: (Employee) );
```

## DROP DEMO USER

```sql
DROP DEMO USER '<username>';
```

Example:

```sql
DROP DEMO USER 'demo_admin';
```

## Enabling Demo Users

Demo users only appear on the login screen when enabled in project security:

```sql
ALTER APP SECURITY ( EnableDemoUsers: TRUE );
```

To hide them:

```sql
ALTER APP SECURITY ( EnableDemoUsers: FALSE );
```

## Listing Demo Users

```sql
LIST DEMO USERS;
```

## Typical Setup

```sql
-- Enable demo users and set prototype security
ALTER APP SECURITY ( SecurityLevel: PROTOTYPE );
ALTER APP SECURITY ( EnableDemoUsers: TRUE );

-- Create demo accounts for each role
CREATE DEMO USER 'demo_admin' ( Password: 'Admin123!', Entity: Administration.Account, UserRoles: (Administrator) );
CREATE DEMO USER 'demo_user' ( Password: 'User1234!', Entity: Administration.Account, UserRoles: (Employee) );
CREATE DEMO USER 'demo_guest' ( Password: 'Guest123!', Entity: Administration.Account, UserRoles: (Guest) );
```

## See Also

- [Security](./security.md) -- overview of the security model
- [Module Roles and User Roles](./roles.md) -- defining the user roles assigned to demo users
- [GRANT / REVOKE](./grant-revoke.md) -- complete permission management reference
