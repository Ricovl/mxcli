# REFRESH CATALOG

## Synopsis

    REFRESH CATALOG [ FULL ] [ FORCE ]

## Description

Rebuilds the project metadata catalog from the current state of the open project. The catalog is a SQLite database (`.mxcli/catalog.db`) that enables fast SQL querying and cross-reference navigation over project elements.

Without any options, `REFRESH CATALOG` performs a basic rebuild that populates the core metadata tables: MODULES, ENTITIES, ATTRIBUTES, ASSOCIATIONS, MICROFLOWS, NANOFLOWS, PAGES, SNIPPETS, ENUMERATIONS, and WORKFLOWS.

With the `FULL` option, the rebuild additionally populates cross-reference tables (REFS), widget inventories (WIDGETS), string tables (STRINGS), source text (SOURCE), activity tables (ACTIVITIES), and permission tables (PERMISSIONS). The FULL rebuild is required before using `LIST CALLERS`, `LIST CALLEES`, `LIST REFERENCES`, `LIST IMPACT`, `DESCRIBE CONTEXT`, or `SEARCH`.

The catalog is cached between sessions. If the project has not changed, repeated `REFRESH CATALOG` calls reuse the cached data. Use `FORCE` to bypass the cache and rebuild unconditionally -- useful after external changes to the MPR file.

## Parameters

**FULL**
: Include cross-references, widgets, strings, source text, and permissions in the catalog. Required for callers/callees analysis and full-text search.

**FORCE**
: Bypass the catalog cache and force a complete rebuild regardless of whether the project appears unchanged.

## Examples

### Basic catalog refresh

```sql
REFRESH CATALOG;
```

### Full rebuild with cross-references

```sql
REFRESH CATALOG FULL;
```

### Force a complete rebuild

```sql
REFRESH CATALOG FULL FORCE;
```

### Refresh from the command line

```sql
-- Shell commands:
-- mxcli -p app.mpr -c "REFRESH CATALOG"
-- mxcli -p app.mpr -c "REFRESH CATALOG FULL"
-- mxcli -p app.mpr -c "REFRESH CATALOG FULL FORCE"
```

## See Also

[SELECT FROM CATALOG](select-from-catalog.md), [LIST CATALOG TABLES](list-catalog-tables.md), [LIST CALLERS / CALLEES](list-callers-callees.md)
