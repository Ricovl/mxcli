# REVOKE

## Synopsis

```sql
-- Entity access (full -- removes entire rule)
REVOKE ALL ON ENTITY module.Entity FROM module.Role [, ...]

-- Entity access (partial -- downgrades specific rights)
REVOKE rights ON ENTITY module.Entity FROM module.Role [, ...]

-- Microflow access
REVOKE EXECUTE ON MICROFLOW module.Name FROM module.Role [, ...]

-- Page access
REVOKE VIEW ON PAGE module.Name FROM module.Role [, ...]

-- Nanoflow access
REVOKE EXECUTE ON NANOFLOW module.Name FROM module.Role [, ...]
```

## Description

Removes previously granted access rights from module roles. Each form is the counterpart to the corresponding GRANT statement.

### Entity Access

The entity revoke mirrors the entity grant: rights first, then `ON ENTITY`, then the roles after `FROM`. `REVOKE ALL` removes the entire entity access rule for the specified module roles on the entity. The older form with the role first, `REVOKE module.Role ON module.Entity [( rights )]`, still parses and warns `MDL-DEPR082`; `mxcli fmt --upgrade` rewrites it.

With a rights list, performs a **partial revoke**: `REVOKE READ (x)` sets member x to no access. `REVOKE WRITE (x)` downgrades member x from ReadWrite to ReadOnly. `REVOKE CREATE` and `REVOKE DELETE` remove the structural permission. The access rule itself is preserved.

### Microflow Access

Removes execute permission on a microflow from one or more module roles.

### Page Access

Removes view permission on a page from one or more module roles.

### Nanoflow Access

Removes execute permission on a nanoflow from one or more module roles.

## Parameters

`module.Role`
:   The module role losing access. Must be a qualified name (`Module.RoleName`).

`module.Entity`
:   The entity whose access rule is removed or modified.

`rights`
:   Optional. A comma-separated list of rights to revoke (partial revoke). Same syntax as GRANT rights:
    - `CREATE` -- revoke create permission
    - `DELETE` -- revoke delete permission
    - `READ *` -- revoke all read access
    - `READ (Attr1, ...)` -- revoke read on specific attributes
    - `WRITE *` -- downgrade all members from ReadWrite to ReadOnly
    - `WRITE (Attr1, ...)` -- downgrade specific attributes from ReadWrite to ReadOnly

`module.Name`
:   The target microflow, nanoflow, or page.

`FROM module.Role [, ...]`
:   One or more module roles losing access (for microflow, page, and nanoflow forms).

## Examples

Remove all entity access for a role:

```sql
REVOKE ALL ON ENTITY Shop.Customer FROM Shop.Viewer;
```

Partial revoke -- remove read access on a specific attribute:

```sql
REVOKE READ (Notes) ON ENTITY Shop.Customer FROM Shop.User;
```

Partial revoke -- downgrade write to read-only:

```sql
REVOKE WRITE (Email) ON ENTITY Shop.Customer FROM Shop.User;
```

Partial revoke -- remove structural permission:

```sql
REVOKE DELETE ON ENTITY Shop.Customer FROM Shop.User;
```

Remove microflow execution from multiple roles:

```sql
REVOKE EXECUTE ON MICROFLOW Shop.ACT_Order_Process FROM Shop.Viewer;
```

Remove page visibility:

```sql
REVOKE VIEW ON PAGE Shop.Admin_Dashboard FROM Shop.User;
```

Remove nanoflow execution:

```sql
REVOKE EXECUTE ON NANOFLOW Shop.NAV_ValidateInput FROM Shop.Viewer;
```

## See Also

[GRANT](grant.md), [CREATE MODULE ROLE](create-module-role.md)
