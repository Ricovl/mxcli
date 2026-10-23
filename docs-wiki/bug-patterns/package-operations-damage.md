---
title: Package Operations That Damage the Project
category: bug-pattern
last-synced: ee7169ba
covers:
  - cmd/mxcli
  - cmd/mxcli/marketplace
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - cmd/mxcli/marketplace/update.go
  - cmd/mxcli/marketplace/workdir.go
  - cmd/mxcli/marketplace/scratch.go
  - docs-site/src/guides/marketplace.md
---

> **Do not duplicate**: what `marketplace install`, `update` and `diff` do — what
> they install, where they build references, what they cache — lives in
> `docs-site/src/guides/marketplace.md` and the CLI help; the drift-detection
> design in `PROPOSAL_marketplace_module_upgrade.md`; the per-operation repairs
> and their measurements in the findings. This page describes the shared hazard.

## What this is

Installing, updating or diffing a marketplace module does two risky things. It
hands the project to tools mxcli does not control (`mx module-import`, `mx
update-widgets`, `mx check`, a build), and it copies files out of a package mxcli
did not write. Both halves have changed things nobody asked them to change, and
the damage is usually reported as success: `mx check` clean, exit 0, a summary of
what was copied.

## How it fits

**The v2 → v1 collapse is the worst of them.** `mx module-import`, `mx
update-widgets` and `mx rename-design-properties` each rewrite an MPR v2 project
(`.mpr` plus a `mprcontents/` tree) as a single-file v1 database. `mx check`
afterwards reports **0 errors**, while `mprcontents/` is gone, the `.mpr` has
grown from tens of kilobytes to tens of megabytes, and per-document diffing no
longer works. Two repairs exist, and which one fits depends on whether the tool's
result has to persist. If the result only has to last for one check, snapshot the
storage and restore it afterwards. If the result must persist, harvest it: let
the tool convert, read the units back, restore v2, and write only the changed
ones through mxcli's own writer. The collapse also came back once through a help
string rather than a call: a "Next:" hint told the user to run the bare tool that
every code path had been guarded against. The reverse holds too. A step every
install needs, printed as "Next" rather than run, is skipped by agents and humans
alike.

**Every mx tool writes into the project it is given — a check and a build too.**
The protection was added one call site at a time, and each time it covered only
the files someone expected to change. A v1 `.mpr` was rewritten permanently, and
a plain `mx check` wrote `theme-cache/` and `deployment/sass/` from four different
callers. What found them was a whole-tree hash+mtime diff before and after, not a
check of the expected files. The fix that holds is a temporary copy for any mx
run that is not *meant* to change the project (`docker.MxCheckOnCopy`,
`copyProjectToTemp`). A new `exec.Command(mx…, project)` needs that decision made
explicitly: expected write, or copy.

**The package says what it ships — take its word, not a guess.** The
bundled-file step was wrong three times in a row, and each time it was a
plausible guess:

1. It copied the model alone, and the old widget binaries kept running.
2. It added `widgets/`, but the module's own `design-properties.json` stayed
   old, giving CE6083 that `rename-design-properties` cannot clear.
3. It copied everything except `project.mpr` and `package.xml`, which put the
   publisher's `License.txt`, `manifest.json` and OSS report into the app root.

The answer was in the package the whole time: `package.xml` lists its project
files under `<files>`. One `mx module-import` with a listing taken before and
after confirmed that mx writes exactly that list, and that it passes over a
listed file the archive lacks. When the question is "what does Mendix do with
this input", one run of Mendix's own tool against such a listing settles it;
enumerating what seems to matter does not. One test had even used
`manifest.json` as its "legitimate entry", so it treated the third bug as
expected behaviour. Read an old fixture before trusting it as a baseline.

**Within what the package ships, the project's newer copy wins.** A module
bundles a copy of every widget its pages use, pinned at its author's release
time, and different modules pin the same widget at different versions. Installing
one module rolled a project's widgets from 3.11.3 back to 3.4.0, with `mx check`
clean throughout. Anything that copies bundled files has to compare versions
before overwriting. It also has to *report* what it skipped: "the package wanted a
different version" is exactly the fact that was invisible.

**A model that builds can still be running the old code.** After an update the
model referenced new widget definitions while `widgets/` held the old binaries,
and `show modules` reported the new version. The model and the deployed artefacts
are separate, and `mx check` inspects only one of them. A module with no widgets
cannot expose this, which is how the gap survived a full end-to-end test on
Administration.

**Replacing a module is a delete, and a delete cascades.** Dropping the old
module removes its entities. Deleting an entity also removes, in every other
module, the associations that point at it. An update therefore destroyed
associations that the user's own module owned, and no module-scoped enumeration
could see or save them. The question to ask of a replace is what deleting the
module cascades into, not what the module contains.

The arriving half has its own gap. A transplant is the one write path that copies
units verbatim, so a package's stale access rules reach the project as CE0066.
Reconcile on arrival, but only where needed: rewriting an untouched marketplace
module makes the next `diff` read it as a local edit, and that edit makes the next
update refuse. [[access-rule-reconciliation]] covers the rule itself.

**Order the steps so a failure is not half an install.** The model is written
before the bundled files are copied. So when a relative `-p` broke the file
step's containment guard, the command exited 1 over a module the model already
had. A guard has two inputs and both must be canonical: the entry *and* the root.
Run cheap, reversible steps first, or say in the error that the model has already
changed.

**Version numbers are not identity.** Matching an installed module against a
marketplace release by version *number* is ambiguous: a blank project ships two
different modules that both published a 4.1.0. The module's `AppStoreGuid` is the
version UUID and is the thing to match on.

**Report what could not be determined, never "unchanged".** Drift detection
compares the installed module against a freshly built reference, so the
reference's blind spots become false claims about the user's project. Three
shapes recur:

- **A describe can succeed and say nothing.** An empty body, or a read-only
  building block, describes successfully. When two such renderings differ, the
  difference was reported as an edit, and the `--save-edits` file that "rescued"
  it would have emptied the element on replay. Compare for equality first, then
  ask whether the text says anything.
- **A placeholder is not content.** The reference cache dropped the action
  sources that DESCRIBE reads, so on a cache *hit* every JavaScript action read as
  edited, while the first, missed run was clean. A bug that appears only on the
  second run needs a miss-then-hit test.
- **The same report must say when "no modifications found" is not a
  conclusion.**

**mx's limits are mxcli's to respect in its own scratch space.** A reference
project is built by mx, in a directory mxcli chooses. mx refuses any file path of
260 characters or more, on every platform: the check is Mendix's own, not
Windows'. Bundled `node_modules` trees run to about 200 characters, so the choice
of directory decides whether `diff` and `update` can run at all. It was filed as
a Windows environment failure. It reproduced on Linux the first time mx ran in a
deep scratch directory, and that was the tell that the directory was mxcli's
choice and the bug was mxcli's.

The shortest directory is not automatically the right one. A folder at a Windows
drive root is modifiable by every local account, and the downloaded package sits
there between download and import. The rule is the shortest root the *user*
owns. The scratch space has a second trap: format is detected by adjacency, so a
package's `project.mpr` unpacked beside a scratch v2 project reads against the
*other* project's units. That failure is silent in both directions. Unpack every
foreign `.mpr` into a directory of its own.

**Environment failures masquerade as project corruption.** Each of these left
output that looks like a broken install:

- an `.mpk` built by a newer Studio Pro: the API already says
  `minSupportedMendixVersion`, so check it before downloading;
- an installed version that has since been unpublished, so the baseline cannot
  be built and `--force` has no finding to override;
- an `mx` resolving to a different cached Mendix version.

The remedies are mostly diagnosis: say which version, which path, which binary.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  repairs and their measurements
- [Marketplace guide](../../docs-site/src/guides/marketplace.md) — what the
  commands install, where references are built, and what is cached
- [[mpr-read-write]] — what v1 and v2 actually are, and why the collapse matters
- [[access-rule-reconciliation]] — why a transplanted module's rules need
  reconciling, and why only where they are stale
- [[element-identity]] — why an update carries GUIDs across the replace
- `PROPOSAL_marketplace_module_upgrade.md` — the drift-detection design
