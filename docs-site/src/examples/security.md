# Role-Based Security

A complete security setup: module roles, entity access with XPath row-level constraints, document access, user roles, and demo users.

## Module Roles

Module roles define what actions are available within a module:

```sql
CREATE MODULE ROLE Sales.Viewer DESCRIPTION 'Read-only access to sales data';
CREATE MODULE ROLE Sales.User DESCRIPTION 'Can create and edit orders';
CREATE MODULE ROLE Sales.Admin DESCRIPTION 'Full access including delete';
```

## Entity Access

GRANT controls which CRUD operations a role can perform. XPath constraints in `WHERE` filter which rows are visible:

```sql
-- Admin: full access to all customers
GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Sales.Customer TO Sales.Admin;

-- User: can create and edit, but only active customers
GRANT CREATE, READ *, WRITE * ON ENTITY Sales.Customer TO Sales.User
  WHERE [IsActive = true];

-- Viewer: read-only, active customers only
GRANT READ * ON ENTITY Sales.Customer TO Sales.Viewer
  WHERE [IsActive = true];

-- Orders: users can only see their own (via owner token)
GRANT CREATE, READ *, WRITE * ON ENTITY Sales.Order TO Sales.User
  WHERE [System.owner = '[%CurrentUser%]'];

-- Admin sees all orders
GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Sales.Order TO Sales.Admin;
```

## Microflow and Page Access

```sql
-- Microflow access
GRANT EXECUTE ON MICROFLOW Sales.ACT_Order_Save TO Sales.User;
GRANT EXECUTE ON MICROFLOW Sales.ACT_Order_Delete TO Sales.Admin;

-- Page access
GRANT VIEW ON PAGE Sales.Customer_Overview TO Sales.Viewer;
GRANT VIEW ON PAGE Sales.Customer_Overview TO Sales.User;
GRANT VIEW ON PAGE Sales.Order_Edit TO Sales.User;
GRANT VIEW ON PAGE Sales.Admin_Dashboard TO Sales.Admin;
```

## User Roles

User roles combine module roles from different modules into a single assignable role:

```sql
CREATE OR MODIFY USER ROLE SalesViewer (System.User, Sales.Viewer);
CREATE OR MODIFY USER ROLE SalesRep (System.User, Sales.User);
CREATE OR MODIFY USER ROLE SalesManager (System.User, Sales.Admin) MANAGE ALL ROLES;
```

## Demo Users

Demo users are created for testing and development:

```sql
CREATE OR MODIFY DEMO USER 'viewer' PASSWORD 'Password1!' (SalesViewer);
CREATE OR MODIFY DEMO USER 'sales_rep' PASSWORD 'Password1!' (SalesRep);
CREATE OR MODIFY DEMO USER 'manager' PASSWORD 'Password1!' (SalesManager);

-- Enable demo users in project security
ALTER PROJECT SECURITY DEMO USERS ON;
```

## Additive Grants

GRANT merges with existing access — it never removes permissions:

```sql
-- Viewer already has READ (Name, Email)
GRANT READ (Phone) ON ENTITY Sales.Customer TO Sales.Viewer;
-- Result: READ (Name, Email, Phone)
```

## Revoking Access

```sql
-- Remove all access for a role
REVOKE Sales.Viewer ON Sales.Customer;

-- Partial revoke: remove read on a specific attribute
REVOKE Sales.User ON Sales.Customer (READ (Phone));

-- Partial revoke: downgrade write to read-only
REVOKE Sales.User ON Sales.Customer (WRITE (Email));

-- Remove microflow access
REVOKE EXECUTE ON MICROFLOW Sales.ACT_Order_Delete FROM Sales.User;
```
