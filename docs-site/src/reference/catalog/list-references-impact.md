# LIST REFERENCES / IMPACT, DESCRIBE CONTEXT

## Synopsis

    LIST REFERENCES TO qualified_name

    LIST IMPACT OF qualified_name

    DESCRIBE CONTEXT OF qualified_name [ DEPTH n ]

## Description

These commands provide different views of cross-reference information for a given element. All three require `REFRESH CATALOG FULL` to have been run beforehand.

**LIST REFERENCES TO** lists all elements that reference the specified element. This includes microflows that use an entity, pages that display it, associations that connect to it, and any other form of reference.

**LIST IMPACT OF** performs an impact analysis showing what would be affected if the specified element were changed or removed. This is broader than `LIST REFERENCES` as it considers transitive dependencies and indirect effects.

**DESCRIBE CONTEXT OF** assembles the surrounding context of an element -- its definition, its callers, callees, and related elements -- suitable for providing to an LLM or for understanding an element in its broader project context. The optional `DEPTH` parameter controls how many levels of related elements to include.

### Attributes, enumerations and enumeration values

The target may also be an attribute (`Module.Entity.Attribute`), an enumeration, or an enumeration value (`Module.Enum.Value`). The graph records:

| Kind | Meaning |
|------|---------|
| `member` | a microflow, nanoflow, rule, page, snippet, workflow or import/export mapping binds, reads or writes the attribute, or navigates the association |
| `xpath` | an XPath constraint names the attribute or association, or compares an enumeration attribute with the value |
| `type` | an attribute, parameter or variable is typed as the enumeration |
| `value` | an expression names the enumeration value |
| `mapping` | an import or export mapping maps the entity |

`IMPACT OF` an enumeration includes the uses of each of its values, with a `Target` column naming which one.

Results list each (source, kind) once, and the `IMPACT` summary counts distinct elements.

When nothing is found, the message says what was searched. An attribute named only through a variable in a free-text expression (`$Order/Total`), and an enumeration value used only as a decision branch, are not resolved by the catalog, so an empty result for an attribute or a value tells you to run `SEARCH` before treating it as unused.

## Parameters

**qualified_name**
: The fully qualified name of the element to analyze (e.g., `Module.EntityName`, `Module.MicroflowName`, `Module.Entity.Attribute`, `Module.Enum.Value`).

**n** (CONTEXT only)
: The number of levels of related elements to include. Defaults to 1 if not specified. Higher values include more surrounding context but produce more output.

## Examples

### Find all references to an entity

```sql
REFRESH CATALOG FULL;
LIST REFERENCES TO Sales.Customer;
```

### Analyze impact before making changes

```sql
LIST IMPACT OF Sales.Customer;
```

### Check an attribute before dropping it

```sql
LIST IMPACT OF Sales.Order.DiscountCode;
```

### Gather context for a microflow

```sql
DESCRIBE CONTEXT OF Sales.ACT_CreateOrder;
```

### Gather deeper context

```sql
DESCRIBE CONTEXT OF Sales.ACT_CreateOrder DEPTH 3;
```

### Check impact before moving an element

```sql
LIST IMPACT OF Sales.CustomerEdit;
MOVE PAGE Sales.CustomerEdit TO NewModule;
```

### From the command line

```sql
-- Shell commands:
-- mxcli refs -p app.mpr Sales.Customer
-- mxcli impact -p app.mpr Sales.Customer
-- mxcli context -p app.mpr Sales.ACT_CreateOrder --depth 3
```

## See Also

[LIST CALLERS / CALLEES](list-callers-callees.md), [REFRESH CATALOG](refresh-catalog.md), [SELECT FROM CATALOG](select-from-catalog.md)
