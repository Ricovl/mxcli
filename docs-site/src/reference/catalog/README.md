# Catalog Statements

The catalog is a SQLite database that caches project metadata for fast querying and cross-reference navigation. It is stored in `.mxcli/catalog.db` next to the MPR file.

A basic `REFRESH CATALOG` populates tables for modules, entities, attributes, associations, microflows, nanoflows, pages, snippets, enumerations, and workflows. A `REFRESH CATALOG FULL` additionally populates cross-references (REFS), widgets, strings, source text, and permissions -- enabling callers/callees analysis, impact analysis, and full-text search.

| Statement | Description |
|-----------|-------------|
| [REFRESH CATALOG](refresh-catalog.md) | Rebuild the catalog from the current project state |
| [SELECT FROM CATALOG](select-from-catalog.md) | Query catalog tables with SQL syntax |
| [LIST CATALOG TABLES](list-catalog-tables.md) | List available catalog tables and their columns |
| [LIST CALLERS / CALLEES](list-callers-callees.md) | Find what calls an element or what it calls |
| [LIST REFERENCES / IMPACT / CONTEXT](list-references-impact.md) | Cross-reference navigation and impact analysis |
