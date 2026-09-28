# Security Statements

Statements for managing project security: module roles, user roles, entity access rules, microflow and page access, demo users, and project-level security settings.

Mendix security operates at two levels. **Module roles** define permissions within a single module (entity access, microflow execution, page visibility). **User roles** aggregate module roles into project-wide identities assigned to end users.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE MODULE ROLE](create-module-role.md) | Create a role within a module |
| [CREATE USER ROLE](create-user-role.md) | Create a project-level user role aggregating module roles |
| [GRANT](grant.md) | Grant entity, microflow, page, or nanoflow access to roles |
| [REVOKE](revoke.md) | Remove previously granted access |
| [CREATE DEMO USER](create-demo-user.md) | Create a demo user for development and testing |
| [UPDATE SECURITY](update-security.md) | Re-sync entity access rules with the domain model (CE0066) |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Describe app security | `DESCRIBE APP SECURITY` |
| Show module roles | `LIST MODULE ROLES [IN module]` |
| Show user roles | `LIST USER ROLES` |
| Show demo users | `LIST DEMO USERS` |
| Show access on element | `LIST ACCESS ON [ENTITY\|MICROFLOW\|PAGE\|NANOFLOW] module.Name` |
| Describe security matrix | `DESCRIBE SECURITY MATRIX [IN module]` |
| Alter app security level | `ALTER APP SECURITY ( SecurityLevel: OFF\|PROTOTYPE\|PRODUCTION )` |
| Toggle demo users | `ALTER APP SECURITY ( EnableDemoUsers: TRUE\|FALSE )` |
| Toggle strict mode | `ALTER APP SECURITY ( StrictMode: TRUE\|FALSE )` |
| Toggle guest access | `ALTER APP SECURITY ( EnableGuestAccess: TRUE\|FALSE [, GuestUserRole: UserRole] )` |
| Drop module role | `DROP MODULE ROLE module.Role` |
| Drop user role | `DROP USER ROLE [IF EXISTS] Name` |
| Drop demo user | `DROP DEMO USER [IF EXISTS] 'username'` |
| Alter user role | `ALTER USER ROLE Name ADD\|DROP MODULE ROLES (module.Role, ...)` |
