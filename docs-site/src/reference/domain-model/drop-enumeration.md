# DROP ENUMERATION

## Synopsis

    DROP ENUMERATION module.name

## Description

`DROP ENUMERATION` removes an enumeration type from the project. Any entity attributes that reference the enumeration as their type (via `Enumeration(Module.EnumName)`) will become invalid after the enumeration is dropped.

Use `LIST IMPACT OF Module.EnumName` to check which entities and attributes reference the enumeration before dropping it.

## Parameters

**module.name**
: The qualified name of the enumeration to remove, in the form `Module.EnumerationName`.

## Examples

### Drop an enumeration

```sql
DROP ENUMERATION Sales.OrderStatus;
```

### Drop after checking references

```sql
LIST IMPACT OF Sales.OldStatus;
DROP ENUMERATION Sales.OldStatus;
```

## See Also

[CREATE ENUMERATION](create-enumeration.md)
