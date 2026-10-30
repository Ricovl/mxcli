# REFRESH CATALOG

The `REFRESH CATALOG` command builds or rebuilds the catalog index from the current project state. The catalog must be refreshed before running catalog queries or cross-reference navigation commands.

## Syntax

```sql
-- Basic refresh: builds metadata tables
REFRESH CATALOG;

-- Full refresh: builds metadata + cross-references + source
REFRESH CATALOG FULL;

-- Force rebuild: ignores cached catalog
REFRESH CATALOG FULL FORCE;
```

## Refresh Levels

### Basic Refresh

```sql
REFRESH CATALOG;
```

Builds the core metadata tables: entities, attributes, associations, microflows, pages, enumerations, and other element types. This is sufficient for structural queries like "list all entities in a module" or "find pages with a specific URL."

### Full Refresh

```sql
REFRESH CATALOG FULL;
```

Builds everything in the basic refresh plus:

- **Cross-reference data** -- Which elements reference which other elements
- **Source content** -- Microflow activity content, expression strings, widget properties
- **Call graph** -- Caller/callee relationships between elements

This level is required for:
- `LIST CALLERS OF`
- `LIST CALLEES OF`
- `LIST REFERENCES TO`
- `LIST IMPACT OF`
- `DESCRIBE CONTEXT OF`
- `SEARCH`
- Full-text catalog queries

### Force Rebuild

```sql
REFRESH CATALOG FULL FORCE;
```

Ignores the cached `.mxcli/catalog.db` file and rebuilds from scratch. Use this when the catalog appears stale or corrupt.

## CLI Usage

```bash
# Quick refresh
mxcli -p app.mpr -c "REFRESH CATALOG"

# Full refresh with cross-references
mxcli -p app.mpr -c "REFRESH CATALOG FULL"

# Force rebuild
mxcli -p app.mpr -c "REFRESH CATALOG FULL FORCE"
```

## Caching

The catalog is cached in `.mxcli/catalog.db` next to the MPR file. On subsequent refreshes, mxcli checks whether the project has changed and only rebuilds if necessary (unless `FORCE` is specified).

A catalog with the MDL source index (`REFRESH CATALOG FULL SOURCE`) keeps it on every later rebuild, including the ones `SEARCH` and `SHOW REFERENCES` start when the project has changed. Those rebuilds describe again only the documents that changed, and the documents that reference them. An entity counts as changed when its own definition does, not when another entity of its module does; a change to an association or annotation describes every entity of that module again. Every other row is carried over from the cache, so a rebuild after one edit costs about as much as `REFRESH CATALOG FULL`. Everything is described again when something outside the described documents changes (a folder, a module, the project settings, a Java action), when another mxcli build wrote the cache, and with `FORCE`.

### The unit store (MPR v2)

An MPR v2 project keeps each document in its own file under `mprcontents/`, and a refresh reads all of them. mxcli keeps a compressed copy of those files in `.mxcli/cache/unit-contents.db` and reads a document from its own file only when the file has changed since mxcli last read it (its size or modification time differs, or the `.mpr` records another content hash). A refresh after a save therefore opens one or two files instead of thousands. That matters most on Windows, where a virus scanner inspects every file a process opens.

The store is a cache: deleting it costs one slower refresh, and a store that cannot be read or written is ignored. It is about half the size of `mprcontents/`, and its directory has its own `.gitignore`, so it is never committed even where `.mxcli/` is not ignored.

## When to Refresh

- **After opening a project** -- Run `REFRESH CATALOG` to enable queries
- **Before cross-reference navigation** -- Run `REFRESH CATALOG FULL` to enable CALLERS/CALLEES/IMPACT
- **After making changes** -- Refresh to update the catalog with new elements
- **If results seem stale** -- Use `REFRESH CATALOG FULL FORCE` to force a rebuild

> **FULL mode populates the analytic tables.** Plain `REFRESH CATALOG` (fast mode)
> leaves `CATALOG.ACTIVITIES`, `CATALOG.REFS`, `CATALOG.XPATH_EXPRESSIONS`, and
> `CATALOG.WIDGETS` **empty** — a fast-mode query against them warns *"requires
> refresh catalog full"*. Run `REFRESH CATALOG FULL` to fill them (activity rows
> carry the model GUIDs, cross-reference edges, and XPath expressions).
