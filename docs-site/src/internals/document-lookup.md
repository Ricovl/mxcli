# Document Lookup: Why DESCRIBE Was Quadratic

Exporting a whole project to MDL is a sweep of `describe` statements — one per
document, usually generated from `show microflows`, `show pages`, and friends, or
produced wholesale by `describe module <Name> with all`. On a large app this used
to take hours. This page explains why, and what the fix constrains you to.

## The shape of the problem

Every describe handler resolved its target the same way:

```go
allPages, err := ctx.Backend.ListPages()   // decodes EVERY page in the project
for _, p := range allPages {
    if p.Name == name.Name && modName == name.Module {
        foundPage = p
        break
    }
}
```

`List*` is not a cheap index read. It walks every unit of that BSON `$Type`,
BSON-decodes each one into a `gen` element, and converts each to the semantic
model. So describing one document costs a full decode of its whole type, and
describing all N documents of a type costs N full decodes — quadratic in the size
of the project, for a job whose theoretical minimum is one decode per document.

Measured on a 23,132-unit app with 10,001 microflows and 780 pages:

| | per describe | all of them |
|---|---|---|
| microflow, list-and-scan | ~1.1s | ~3 hours |
| microflow, by-name index | ~0.4ms | 4.3s |
| page, list-and-scan | ~26ms | 20.9s |
| page, memoized list | ~2.8ms | 2.8s |

## Two fixes, because there are two kinds of document type

Which one applies depends on how heavy the type's `gen`→model conversion is —
**not** on how many documents there are.

**Header types** — pages, enumerations, layouts, constants, Java actions,
mappings, and most others. `pageFromGen` and its siblings keep identity and a few
header fields; a page's widget tree is *not* retained (DESCRIBE PAGE re-reads it
separately, by ID). Measured: `show pages`, `show layouts`, `show enumerations`
all peak at the same ~340MB as the bare unit cache. The decode is the cost, and
the decode is worth memoizing.

These go through [`mprread.ListUnitsWithContainerCached`], which memoizes the
decoded slice on the `Reader` until the next `InvalidateCache` — i.e. until any
write. Call sites opt in, and two rules govern which may:

1. **Read-only callers only.** The elements are shared with every later caller,
   so mutating one in place poisons the memo. The OQL rewrite in
   `move_view_write.go` does exactly that, and stays on the uncached function.
2. **Not for types whose decoded form is large.** See below.

**Flow types** — `Microflow`, `Nanoflow`, `Rule`. The conversion keeps the entire
flow, so retaining every decoded microflow costs **~554MB** on a 10k-microflow
app, against that same ~340MB floor. Memoizing these is the wrong trade.

These resolve one document at a time through [`mprread.GetUnitByName[T]`], which
looks the qualified name up in the `Reader`'s lightweight header index — built
once, from BSON headers only, no nested content decoded — and then decodes
exactly the one unit asked for.

## The header index is type-agnostic; the alias table is not

`mpr.Reader.GetUnitByName` takes a human-friendly alias (`"microflow"`, `"page"`)
and resolves it through `rawUnitBSONType`, a hand-maintained table covering about
a dozen types. `mprread.GetUnitByName[T]` derives the storage name from the codec
registry the same way `ListUnitsWithContainer[T]` does, so it works for every
registered type without anyone remembering to extend a table. Prefer it wherever
the concrete `gen` type is known. Both reach the same index through
`Reader.GetUnitByTypeName`.

## Cost of the memo

Retaining decodes that used to be transient garbage raises peak RSS. On the same
app, `describe module CDD with all` went **11.1s → 2.8s** with peak RSS up ~180MB
(~16%). That is the intended trade. `MXCLI_NO_DECODE_CACHE=1` turns the memo off
for a memory-constrained machine, or to rule it out when bisecting a suspected
stale read.

## If you add a document type

- A read-only `List*` on a header type should call
  `mprread.ListUnitsWithContainerCached[T]`.
- A `Get*ByName` should call `mprread.GetUnitByName[T]` — never list-and-scan.
- A **write** path must call `mprread.ListUnitsWithContainer[T]`, so it never
  mutates a shared element.
- If the type's conversion retains a whole document tree, keep it off the memo
  and resolve it by name instead. Check with `show <type>` and compare peak RSS
  against the ~340MB unit-cache floor.

[`mprread.ListUnitsWithContainerCached`]: https://github.com/mendixlabs/mxcli/blob/main/modelsdk/mprread/generic.go
[`mprread.GetUnitByName[T]`]: https://github.com/mendixlabs/mxcli/blob/main/modelsdk/mprread/generic.go
