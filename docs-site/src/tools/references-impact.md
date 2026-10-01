# LIST REFERENCES / IMPACT

These commands provide broader reference tracking and impact analysis, helping you understand how changes propagate through a project.

## Prerequisites

Both commands require a full catalog refresh:

```sql
REFRESH CATALOG FULL;
```

## LIST REFERENCES TO

Shows all references to and from a given element, combining both incoming and outgoing relationships.

**Syntax:**

```sql
LIST REFERENCES TO <qualified-name>
```

**Examples:**

```sql
-- Find all references to/from an entity
LIST REFERENCES TO Sales.Customer;

-- Find all references to/from a microflow
LIST REFERENCES TO Sales.ACT_ProcessOrder;

-- Find all references to/from a page
LIST REFERENCES TO Sales.CustomerOverview;
```

### CLI Usage

```bash
mxcli refs -p app.mpr Module.Customer
```

### Difference from CALLERS/CALLEES

While `LIST CALLERS` and `LIST CALLEES` focus on the call graph direction, `LIST REFERENCES` combines both directions into a single view. It shows every element that either references or is referenced by the target element.

## LIST IMPACT OF

Performs impact analysis on an element, showing what would be affected if you changed or removed it.

**Syntax:**

```sql
LIST IMPACT OF <qualified-name>
```

**Examples:**

```sql
-- Analyze impact of changing an entity
LIST IMPACT OF Sales.Customer;

-- Analyze impact before removing a microflow
LIST IMPACT OF Sales.ACT_CalculateTotal;

-- Check impact before moving an element
LIST IMPACT OF Sales.CustomerOverview;
```

### CLI Usage

```bash
mxcli impact -p app.mpr Module.Customer
```

## Use Cases

### Pre-Change Impact Assessment

Before modifying an entity, check what would be affected:

```sql
-- What depends on the Customer entity?
LIST IMPACT OF Sales.Customer;

-- Review results: pages, microflows, associations, access rules
-- Then decide if the change is safe to make
```

### Before Moving Elements

Moving elements across modules changes their qualified name and can break references:

```sql
mdl 1;
-- Check impact before moving
LIST IMPACT OF OldModule.Customer;

-- If impact is acceptable, proceed
MOVE ENTITY OldModule.Customer TO NewModule;
```

### Finding All Usages of an Enumeration

```sql
LIST REFERENCES TO Sales.OrderStatus;
-- Shows: entities with attributes of this type, microflows that use it, pages that display it
```

### Dependency Mapping

Understand the full dependency web of a complex module:

```sql
LIST REFERENCES TO Sales.Order;
LIST REFERENCES TO Sales.OrderLine;
LIST REFERENCES TO Sales.Order_Customer;
```
