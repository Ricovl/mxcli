# DESCRIBE ASSOCIATION

## Synopsis

    DESCRIBE ASSOCIATION <qualified_name>

## Description

Shows the complete MDL source for an association, including the FROM entity, TO entity, association type (Reference or ReferenceSet), owner, and delete behavior. The output is round-trippable MDL that can be used directly in a `CREATE ASSOCIATION` statement.

Associations define relationships between entities. A `Reference` type is a one-to-many relationship, while `ReferenceSet` is many-to-many.

## Parameters

*qualified_name*
: A `Module.AssociationName` reference identifying the association to describe. Both the module and association name are required.

## Examples

Describe an association:

```sql
DESCRIBE ASSOCIATION Sales.Order_Customer;
```

Example output:

```sql
/** Links orders to customers */
CREATE ASSOCIATION Sales.Order_Customer
  FROM Sales.Customer
  TO Sales.Order
  TYPE Reference
  OWNER Default
  ON DELETE SET NULL;
```

Describe a many-to-many association:

```sql
DESCRIBE ASSOCIATION Sales.Order_Product;
```

## See Also

[LIST ASSOCIATIONS](list-associations.md), [DESCRIBE ENTITY](describe-entity.md), [LIST MODULES](list-modules.md)
