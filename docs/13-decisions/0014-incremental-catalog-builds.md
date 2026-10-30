# ADR-0014: Catalog rows belong to a unit, and a refresh rebuilds only the changed units

- **Status**: Accepted
- **Date**: 2026-10-30
- **Related**: ako/mxcli#1804 (this decision), #1790 and #1803 (the incremental source index it generalises), #1789 (the unit store)

## Context

The catalog (`.mxcli/catalog.db`) goes stale on every save, and every
command that reads it then rebuilds it. A rebuild produced every table from
every unit: on a 4,753-unit project about 4.5 s of decoding and inserting,
5.6–8 s end to end per refresh. Users reported waits of hours on Windows,
where every unit file a refresh opens is also scanned. Nearly all of that
work reproduces rows that are already in the cache. Only #1790's source
index reused anything, and it still decoded every document to decide.

The builder is about 10,000 lines of passes, written over two years by many
contributors, with about 130 insert statements across 29 files. New passes
will keep arriving. Whatever makes refreshes incremental has to hold for
passes nobody has written yet.

## Decision

Every catalog row belongs to the unit it came from. A wrapper on the build's
transaction records each insert's rowid range under the pass's current owner
(`own(unitID)`), or under no unit for project-wide rows. The next build loads
the cached catalog and plans from the unit content hashes. It rebuilds the
changed units and the units whose `refs` name them, decoding only those, and
drops and rewrites the project-wide rows. Every other row is left as it is.
Anything the plan cannot vouch for makes it a full build: a change to a unit
no per-unit pass owns, another mxcli build or level, or a row written for a
skipped document. An incremental build must produce exactly the rows of a
full one.

## Consequences

- A refresh after one edit on that project takes 0.7 s end to end, against
  5.6–8 s, with or without the source index. Only the rebuilt units' files
  are read.
- **Every new catalog pass carries obligations**: mark its rows' owner, list
  documents through the scoped list, look other documents up in the catalog
  tables, and give any cross-document dependency a `refs` edge. Breaking the
  first two leaves the build correct but slower, and nothing would notice. So
  three tests count the cost: rows with no owner
  (`TestCatalog_RowsBelongToADocument`), documents decoded per incremental
  refresh (`TestCatalog_IncrementalRefreshDecodesOnlyWhatItRebuilds`), and
  direct reader listings (`TestBuilderListsDocumentsThroughTheScope`). Each
  keeps an allowlist that says why its entries are exempt. Breaking the third
  or fourth obligation is a correctness bug, and only the equivalence tests
  can catch it, and only for the steps they include. `/mxcli-dev:review`
  carries the checklist.
- **Correctness depends on `refs` as the model of what a document's rows
  depend on.** A dependency `refs` does not record shows up as a stale row,
  not an error. Two kinds of evidence back it:
  - The equivalence tests compare every table with a forced full build over
    19 kinds of edit, chained incrementally. They cover only what their steps
    exercise.
  - The ref coverage trace (`builder_coverage.go`) records every name the
    cross-document lookups read per document and checks it the way the plan
    would. It must be at 100% on the fixture
    (`TestCatalog_RefsCoverEveryCrossDocumentRead`), and it measures a real
    project on demand (`TestCatalog_RefCoverageOfProject`): 99.4% on two
    large ones when it was added. The gaps it found are refs edges that
    impact analysis lacked too.
  - It sees only reads through the traced lookups, which is why a pass may
    read other documents only through the catalog tables.
- Row order in an unsorted query, and surrogate IDs (`refs.Id`,
  `permissions.Id`), differ between an incremental and a full build. Nothing
  may depend on them.
- The cache now holds bookkeeping (`catalog_row_owners`, `catalog_units`,
  `catalog_unit_names`): about 25,000 ranges on that project.
- Folder, module, settings, security and navigation changes still cost a full
  build. Making one of them incremental means a pass that owns its rows, and
  that every row depending on it is rebuilt with it.

## Alternatives considered

- **Phase-level reuse** (skip a whole pass when none of its unit types
  changed). Simple, but almost every pass reads domain models, so one entity
  edit still rebuilds nearly everything.
- **An owner column on every table.** It changes 78 schemas and 130 inserts,
  shows the bookkeeping in every user query, and a new table can forget it.
  Recording rowid ranges at the transaction needs no change to any insert.
- **Copying the kept rows into a fresh catalog** (the first implementation).
  It was correct, but re-tokenising the carried full-text rows cost more than
  the rest of the build. Starting from the cached catalog and deleting the
  rebuilt rows copies nothing. The same database connection must not be
  loaded twice, though: modernc.org/sqlite crashed once its full-text tables
  had been used, so the plan is made on a scratch copy, and a fallback empties
  the tables instead of reloading them.
- **Trusting the per-unit hash alone** (no dependents). Wrong: a document's
  rows can change while its own unit does not, as in a delete over an
  association whose end moved. The equivalence test proves it with a step
  that fails without the dependents.
