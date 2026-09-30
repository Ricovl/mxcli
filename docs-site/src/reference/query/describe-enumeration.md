# DESCRIBE ENUMERATION

## Synopsis

    DESCRIBE ENUMERATION <qualified_name>

## Description

Shows the complete MDL source for an enumeration, including all values with their captions and any documentation comments. The output is round-trippable MDL that can be used directly in a `CREATE ENUMERATION` statement.

Enumerations define a fixed set of named values used as attribute types on entities.

## Parameters

*qualified_name*
: A `Module.EnumerationName` reference identifying the enumeration to describe. Both the module and enumeration name are required.

## Examples

Describe an enumeration:

```sql
DESCRIBE ENUMERATION Sales.OrderStatus
```

Example output:

```sql
/** Order status enumeration */
CREATE ENUMERATION Sales.OrderStatus (
  Draft 'Draft',
  Pending 'Pending Approval',
  Approved 'Approved',
  Shipped 'Shipped',
  Delivered 'Delivered',
  Cancelled 'Cancelled'
);
```

## See Also

[LIST ENUMERATIONS](list-enumerations.md), [DESCRIBE ENTITY](describe-entity.md), [LIST MODULES](list-modules.md)
