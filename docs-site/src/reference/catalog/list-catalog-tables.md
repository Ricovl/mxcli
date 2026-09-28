# LIST CATALOG TABLES

## Synopsis

    LIST CATALOG TABLES

## Description

Lists all available catalog tables along with their column names and types. This is useful for discovering what metadata is queryable before writing `SELECT FROM CATALOG` queries.

The output includes both basic tables (populated by `REFRESH CATALOG`) and extended tables (populated by `REFRESH CATALOG FULL`). Tables that require a full refresh are marked accordingly.

## Parameters

This statement takes no parameters.

## Examples

### List all catalog tables

```sql
LIST CATALOG TABLES;
```

### Typical workflow: discover then query

```sql
REFRESH CATALOG;
LIST CATALOG TABLES;
SELECT * FROM CATALOG.ENTITIES LIMIT 5;
```

## See Also

[REFRESH CATALOG](refresh-catalog.md), [SELECT FROM CATALOG](select-from-catalog.md)
