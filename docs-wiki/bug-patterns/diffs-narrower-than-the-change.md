---
title: A Diff Is Only as Wide as Its Renderer
category: bug-pattern
last-synced: cf9d5f0c
covers:
  - mdl/executor
sources:
  - .claude/skills/fix-issue/findings/mdl-executor/
  - mdl/executor/cmd_diff_local.go
  - mdl/executor/cmd_diff.go
  - mdl/executor/cmd_diff_mdl.go
---

> **Do not duplicate**: what `diff` and `diff-local` print, and which unit types
> have a renderer, is in `.claude/commands/mendix/diff-local.md` and
> `docs-site/src/tools/diff.md`. The individual fixes are in the findings. This
> page is about why "N modified" with no hunk keeps coming back.

## What this is

`diff-local` does not compare documents. It renders both revisions of a unit as
MDL and compares the **text**. So it can only show a change that its renderer
prints. Whatever the renderer leaves out comes out the same on both sides, and
`Current == Proposed` swallows it. Git, meanwhile, has already counted the file
as modified. The result is the signature of the class:

```
Summary: 0 new, 1 modified, 0 deleted
```

and no hunk. `-f struct` says `(no changes)` under the same count.

This has shipped in five shapes: a renderer stub for a whole doctype (page,
nanoflow and layout bodies), a parse that failed identically on both sides (the
microflow stub when the engine lacked `ParseMicroflowFromRaw`), a property read
under its SDK name instead of its storage name (attribute `Type` vs `NewType`),
a doctype with no case at all (project settings, constants, module security),
and a part of a rendered doctype that was never printed (entity access rules).

## How it fits

**A placeholder that does not vary with its input erases the change it stands
for.** Each stub (`-- (page widgets)`, `-- parse failed --`, the generic
`-- <Type>: <name>` header) was written to degrade visibly. Both sides degrade
the same way, so the comparison sees nothing. The same trap applies to masking
a secret: a fixed `'***'` hides a changed value as completely as no renderer at
all. A mask for a diff must differ where the values differ and still give
nothing away. The settings renderer renders each side knowing the other's value,
so a changed constant prints as `<masked: old value>` / `<masked: new value>`
and an unchanged one as `<masked>`.

**Close the class, not the case.** Adding renderers one doctype at a time (#1080,
#1393, #1602, #1630) never ends, because the next unprinted property is silent
again. Git has already said the bytes differ, so when the two renderings agree
the documents are compared instead, and the unit prints one line naming the
property paths that changed (`ChangedPaths` in `cmd_diff.go`, shared with
`mxcli diff`). It never prints the values, because the doctypes without a
renderer include the ones that hold secrets. The renderers still matter, because
a path list is no review. The note makes sure that a missing renderer is
visible, not silent.

**Decode through the backend, never off the map.** BSON keys are storage names:
an access rule's roles are `AllowedModuleRoles`, an attribute's type is
`NewType`. A hand-written map reader is one wrong key away from printing nothing
on both sides. The `Parse…FromRaw` backend methods decode a `git show` revision
through the codec, and the `describe` renderer then prints it. One renderer then
serves both commands, which is also the remedy for
[duplicate-resolver drift](duplicate-resolver-drift.md).

**The `$Type` dispatch is ordered by containment.** `Microflows$Nanoflow`
contains "Microflow", `Security$ModuleSecurity` contains "Module". A
`strings.Contains` case catches every type whose name includes its needle. Match
an exact type with `==`, and before adding a needle, list which other types
contain it.

**A test for this class must make the documents differ where the renderer is
blind.** A test that changes something the renderer already prints passes
against the bug. A mock parser that accepts any `$Type` hides dispatch faults.
Make mocks refuse foreign documents the way the backend does. The cheapest real
repro is a copy of `testdata/pedapp`, `git init`, exec and commit, then
`diff-local --ref X~1..X`. The pre-fix binary prints the bare summary.

**Where git runs, and what it is asked, are part of the contract.** Git's paths
are relative to the repository root, and git used to run in the process's
working directory. The two only line up when the working directory is the root
and the project sits at it. That is where every early repro ran, so `-p` from
anywhere else failed or read the wrong files. The same goes for the range: the
old side of `a...b` is the merge base, not `a`, and only a repository where both
branches changed the same unit after the split tells the two apart. Rename
detection is a third: a unit's file name is its ID, so a "renamed" file is one
unit deleted and another added.

## See also

- [describe-round-trip-gaps](describe-round-trip-gaps.md): the renderer
  `diff-local` borrows has its own gaps, and every one of them is also a
  diff-local blind spot.
- [duplicate-resolver-drift](duplicate-resolver-drift.md): `diff` once rendered
  its two sides with two different renderers.
- [fix-issue findings](../../.claude/skills/fix-issue/findings/mdl-executor/):
  `grep -l diff-local` for the instances.
