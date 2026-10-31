---
title: Incremental project index — persist the unit index, rebuild the catalog per changed unit
status: superseded
date: 2026-10-14
related:
  - PROPOSAL_agent_loop_efficiency.md
---

# Proposal: Incremental project index

> **Status: Superseded** by upstream's ContentsHash unit index (ako/mxcli#1791)
> and [ADR-0014](../../13-decisions/0014-incremental-catalog-builds.md)
> (incremental catalog builds), which replaced this branch's stat-keyed index
> and UnitId columns.

**Date:** 2026-10-14

## Problem Statement

Exploring a large project means many small reads: `describe`, `refs`,
`callers`, `impact`, `search`, `context`. On a 23,371-unit MPR v2 app (Mendix
10.24, 11,580 microflows), two costs dominate once the per-document work is
fixed:

| Cost | When | Measured |
|---|---|---|
| Header index: every unit file is read to learn its `$Type`, `Name`, container | every mxcli process | ~0.5 s warm, ~8 s with a cold page cache (258 MB in 23,371 files) |
| Full catalog build (`refs`, `search`, `callers`, `impact`, `context` need it) | first catalog command after any change to the `.mpr` | 31 s (112k activities, 111k references, 49k strings), 2.1 GB peak RSS |

A typical trace — references, callers, impact, three describes — runs in
1.4–1.7 s in one process once the catalog exists, so the catalog rebuild after
each save is what a session actually waits on.

There is also a **correctness gap**. The catalog is reused while the `.mpr`
file's mtime is unchanged. On MPR v2 a document's content lives in its own
`mprcontents/` file, so a change that touches only those files — `git pull`,
switching branch, restoring a file — leaves the catalog serving stale answers.
Verified on a fixture copy: renaming a microflow by editing only its unit file,
`show microflows` reports the new name and `select … from CATALOG.MICROFLOWS`
keeps the old one, with no rebuild and no warning. mxcli's own writes do touch
the `.mpr`, so they invalidate correctly.

Both costs and the gap have the same missing piece: knowing **which units
changed** since the last run. Checking every unit file's size and mtime costs
0.04 s on this app, against 0.2 s+ to read them warm and seconds cold.

## BSON Structure

Not applicable: nothing new is read or written in Mendix documents. The index
holds what the reader already extracts per unit (`$ID`, container,
containment name, `$Type`, top-level `Name`, `Excluded`).

## Proposed MDL Syntax

None. Behaviour is automatic; `refresh catalog full force` keeps forcing a
full rebuild, and `MXCLI_NO_PERSISTED_INDEX=1` / `MXCLI_CATALOG_INCREMENTAL=0`
turn each part off (for bisecting a suspected stale read, as
`MXCLI_NO_DECODE_CACHE` does for the decode memo).

## Design

### Change detection (shared)

For MPR v2: the unit set comes from the `.mpr`'s `Unit` table (ID,
container, containment name — a metadata query, no contents), and each unit
file is `stat`ed. A unit is **changed** when it is new, removed, or its file's
size or mtime (nanoseconds) differs from the stored entry, or its container
moved.

*Racily clean* files: a file modified within the timestamp granularity of the
moment the index was written may change again without a visible mtime change
(the problem git's index solves the same way). Entries whose mtime is not
older than the index write time by a safety margin are treated as changed on
the next run.

MPR v1 is out of scope: every change rewrites the single `.mpr`, so today's
mtime check is correct there, and detecting changed rows would mean hashing
every unit's contents — the read this avoids. v1 keeps current behaviour.

### Part A — persisted header index

`modelsdk/mpr` writes the header index (per unit: ID, container, containment
name, type, name, excluded, file size, mtime) to `.mxcli/unit-index` after
building it, by write-to-temp-and-rename so concurrent processes never read a
torn file. On the next open it loads the file, applies the change set, and
re-reads only changed units. `InvalidateCache` after a write takes the same
path, so a write no longer costs a full re-read either.

The index build also reads through the overlay of deferred writes (today it
reads files directly, which skips held writes).

### Part B — incremental catalog

- Every catalog table that holds per-document rows gets a `UnitId` column (the
  owning unit, not the element: an entity's rows belong to its domain model
  unit). Schema version bump; an old cache rebuilds fully once.
- Each per-document builder accepts a unit filter. An incremental refresh
  deletes `WHERE UnitId IN (changed)` from every table, then runs the builders
  for the changed units only.
- Project-wide passes (module graph and cycles, navigation, project security,
  name resolution maps) are recomputed whole; they are expected to be cheap
  next to the per-document builders — to be measured in phase 1.
- **Falls back to a full rebuild** when a module or folder is renamed or moved
  (qualified names ripple into rows of unchanged units), when project settings
  or security change, on a schema or mode change (fast → full → source), or
  when more than a threshold of units changed (e.g. 20%).
- Catalog validity becomes "the change set is empty" instead of the `.mpr`
  mtime, which closes the stale-catalog gap.

### Phase 0 — quick wins, independent of the above

- The catalog builder has `cachedMicroflows()`, but `buildExternalActions` and
  `buildMicroflowPermissions` call `ListMicroflows()` directly: a full build
  decodes all microflows three times (~6–9 s CPU of the 31 s on this app).
- 2.1 GB peak RSS for one build; worth a look once the triple decode is gone.

## Implementation Plan

| Order | Change | Files |
|---|---|---|
| 0 | Builders use `cachedMicroflows()` | `mdl/catalog/builder_external.go`, `builder_permissions.go` |
| 1 | Change set from `Unit` table + `stat`; racily-clean rule | `modelsdk/mpr/reader_units.go` (new `reader_unit_index.go`) |
| 2 | Persist / load the header index; overlay-aware build | `modelsdk/mpr/reader.go`, `reader_units.go` |
| 3 | Catalog validity from the change set (fixes staleness on its own) | `mdl/executor/cmd_catalog.go` |
| 4 | `UnitId` columns, schema bump | `mdl/catalog/tables.go`, every `builder_*.go` insert |
| 5 | Unit filter per builder; delete-and-rebuild; fallback rules | `mdl/catalog/builder*.go`, `cmd_catalog.go` |

Steps 1–3 are shippable alone: faster startup and a correct catalog, still
rebuilding fully. Steps 4–5 make the rebuild incremental.

## Version Compatibility

No Mendix version gating. MPR v2 (Mendix 10.18+) only; v1 unchanged.

## Test Plan

- **Staleness (fails on main):** build the catalog, edit one unit file without
  touching the `.mpr`, query the catalog — must show the change.
- **Persisted index:** a second open of an unchanged project reads 0 unit files
  (`UnitFileReads`); after editing one unit, exactly 1. Racily-clean: a unit
  rewritten within the margin with the same size is re-read. Control: with the
  index disabled, the open reads every unit.
- **Incremental equals full:** for each mutation — edit a microflow, add,
  delete, move between folders, rename a document, rename a module (fallback)
  — dump every catalog table sorted and compare the incremental result with a
  full rebuild. Control: skipping the delete step must make the comparison
  fail.
- **Benchmarks on a large v2 app:** process startup, cold start, catalog
  refresh after a one-microflow edit, full build before/after phase 0.

## Open Questions

- Index file format: `encoding/gob` in `.mxcli/` (proposed) or a table in
  `catalog.db`? Separate keeps the reader layer free of the catalog.
- Studio Pro writing while mxcli reads: is the racily-clean margin enough, or
  should a change set that grows during the refresh restart it?
- Which project-wide passes are actually cheap enough to recompute every time —
  measure before step 5.
- The fallback threshold (20% of units) is a guess; set it from the measured
  cost of an incremental refresh per unit.
