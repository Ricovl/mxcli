# SELECT FROM CATALOG

## Synopsis

    SELECT columns FROM CATALOG.table [ WHERE condition ] [ ORDER BY column [ ASC | DESC ] ] [ LIMIT n ]

## Description

Executes a SQL query against the project metadata catalog. The catalog tables are populated by `REFRESH CATALOG` and can be queried using standard SQL syntax including joins, aggregations, subqueries, and filtering.

All catalog table names are prefixed with `CATALOG.` (e.g., `CATALOG.ENTITIES`, `CATALOG.MICROFLOWS`). The query engine is SQLite, so standard SQLite SQL syntax applies.

The following tables are available after a basic `REFRESH CATALOG`:

| Table | Description |
|-------|-------------|
| `CATALOG.MODULES` | Project modules |
| `CATALOG.ENTITIES` | Entities across all modules |
| `CATALOG.ATTRIBUTES` | Entity attributes |
| `CATALOG.ASSOCIATIONS` | Associations between entities |
| `CATALOG.MICROFLOWS` | Microflows |
| `CATALOG.NANOFLOWS` | Nanoflows |
| `CATALOG.PAGES` | Pages |
| `CATALOG.SNIPPETS` | Snippets |
| `CATALOG.ENUMERATIONS` | Enumerations |
| `CATALOG.WORKFLOWS` | Workflows |

The following tables require `REFRESH CATALOG FULL`:

| Table | Description |
|-------|-------------|
| `CATALOG.ACTIVITIES` | Individual microflow/nanoflow activities |
| `CATALOG.WIDGETS` | Widget instances across pages and snippets |
| `CATALOG.REFS` | Cross-references between elements |
| `CATALOG.PERMISSIONS` | Access rules and role permissions |
| `CATALOG.STRINGS` | All string values in the project |
| `CATALOG.SOURCE` | Source text of microflows, pages, etc. |

## Parameters

**columns**
: Column names or expressions to select. Use `*` for all columns, or specify individual column names. Supports SQL functions like `COUNT()`, `SUM()`, `GROUP_CONCAT()`.

**table**
: A catalog table name (e.g., `ENTITIES`, `MICROFLOWS`). Must be prefixed with `CATALOG.`.

**condition**
: A SQL WHERE clause for filtering rows. Supports standard operators (`=`, `LIKE`, `IN`, `AND`, `OR`) and subqueries.

**column**
: Column to sort by. Append `ASC` (default) or `DESC` for sort direction.

**n**
: Maximum number of rows to return.

## Examples

### List all entities

```sql
SELECT Name, Module FROM CATALOG.ENTITIES;
```

### Find microflows in a specific module

```sql
SELECT Name FROM CATALOG.MICROFLOWS WHERE Module = 'Sales' ORDER BY Name;
```

### Count entities per module

```sql
SELECT Module, COUNT(*) AS EntityCount
FROM CATALOG.ENTITIES
GROUP BY Module
ORDER BY EntityCount DESC;
```

### Find entities with many attributes

```sql
SELECT e.Module, e.Name, COUNT(a.Name) AS AttrCount
FROM CATALOG.ENTITIES e
JOIN CATALOG.ATTRIBUTES a ON e.Id = a.EntityId
GROUP BY e.Module, e.Name
HAVING COUNT(a.Name) > 10
ORDER BY AttrCount DESC;
```

### Find many-to-many associations

```sql
SELECT Name, ParentEntity, ChildEntity
FROM CATALOG.ASSOCIATIONS
WHERE Type = 'ReferenceSet';
```

### Find pages with no microflow references

```sql
SELECT p.Module, p.Name
FROM CATALOG.PAGES p
WHERE p.Id NOT IN (
  SELECT DISTINCT TargetId FROM CATALOG.REFS WHERE RefKind = 'page'
);
```

### Limit results

```sql
SELECT Name FROM CATALOG.MICROFLOWS LIMIT 10;
```

## See Also

[REFRESH CATALOG](refresh-catalog.md), [LIST CATALOG TABLES](list-catalog-tables.md), [SEARCH](list-references-impact.md)
