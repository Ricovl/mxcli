# Diff

mxcli provides two diff commands for comparing MDL scripts against project state and viewing local changes in MPR v2 projects.

## mxcli diff

Shows what `mxcli exec` would change in the project if it ran the script, without
changing the project.

**Usage:**

```bash
mxcli diff -p app.mpr changes.mdl
```

### How it works: diff runs exec

`diff` copies the project to a scratch folder, runs the script there with exec's
own code — the same pre-flight checks, the statements in order, under the
script's language header — and then compares the copy with the project unit by
unit. What it reports is therefore exactly what `exec` would write: there is no
second judgement to disagree with exec's. The project itself is only read, and
the scratch copy is deleted afterwards (it leaves out `.git`, `deployment`,
`releases` and `node_modules`).

Each unit exec would add, rewrite, move or remove is shown as the `describe` of
the document before and after. A domain model is shown per entity and
association, module security per module role, and project security per user
role and demo user. This covers every document kind — pages, snippets,
layouts, navigation, settings, security, folders — and statements of every
kind, including `grant`, `alter`, `move` and `drop`.

```
--- Page.MyFirstModule.Home_Web (current)
+++ Page.MyFirstModule.Home_Web (script)
@@ -1,6 +1,6 @@
 create or modify page MyFirstModule.Home_Web (
   Title: 'Homepage',
-  Layout: Atlas_Core.Atlas_TopBar,
+  Layout: Atlas_Core.Atlas_Default,
   PopupResizable: true
 ) {

Summary: 0 new, 1 modified, 0 removed — exec would write 1 unit(s)
```

A unit exec rewrites although its description does not change — a property
`describe` does not print, a translation, a move to another folder — is listed
on one line with what changes, so a write is never hidden behind identical MDL:

```
Modified: Page MyFirstModule.Home_Web: changed: CanvasHeight, CanvasWidth
Modified: Microflow Shop.ACT_Apply: moved to 'Shop/Archive'
```

Files exec writes next to the model (Java sources, theme files) are listed as
`New file:` / `Modified file:`.

### What diff no longer reports

Because the verdict is exec's own, a script exec has already applied diffs as
`exec would write nothing`. Before (ako/mxcli#907), diff compared the script's
text with the stored document's description and reported phantom changes
exec never makes — `Boolean` against the stored `Boolean default false`,
`String` against `String(unlimited)`, a position, a re-laid-out flow — and
did not compare pages, translations or layouts at all.

### Refusals and errors

- A script exec's pre-flight refuses (a `check` error, an unresolved
  reference, a name clash) is reported as **Refused**, with the same report
  exec prints, and nothing is written. `--no-check` skips the pre-flight, as
  it does for exec.
- A statement exec stops at — a plain `create` of a document that exists
  (ako/mxcli#807), a `create or modify` of a flow the splice cannot make
  under `mdl 1;` — is reported as **Refused** with exec's error, after the
  changes exec makes before it. `--continue-on-error` runs every statement,
  as it does for exec.
- Statements that depend on earlier ones (a flow calling a microflow the
  script creates first) are diffed like any other, because the earlier ones
  run first (ako/mxcli#856).

```
Refused: exec would stop at this error, having written nothing: entity already exists: Shop.Order — …

Summary: 0 new, 0 modified, 0 removed — exec would write nothing
```

`diff` always runs the script with the file engine on the scratch copy, also
under `--mcp`: the script is executed for real, and only the copy may receive
it. `--exec-output` prints what exec reports while it runs.

For the same reason a script may not reach past the copy. A `connect` to the
`-p` project (a headerless script may hold one, and so may a script it runs
with `execute script`) is followed on the copy; a `connect` to any other
project, a `sql <alias> <query>` and an `import from` are refused with an error,
because diff has no copy of that project or database and does not run them for
real.

## mxcli diff-local

Compares local changes against a git reference for MPR v2 projects. MPR v2 (Mendix >= 10.18) stores documents as individual files in an `mprcontents/` folder, making git diff feasible.

**Usage:**

```bash
# Compare against HEAD (latest commit)
mxcli diff-local -p app.mpr --ref HEAD

# Compare against a specific commit
mxcli diff-local -p app.mpr --ref HEAD~1

# Compare against a branch
mxcli diff-local -p app.mpr --ref main

# Compare two arbitrary revisions (git range syntax)
mxcli diff-local -p app.mpr --ref main..feature-branch

# Three-dot range (changes since common ancestor)
mxcli diff-local -p app.mpr --ref main...feature-branch
```

### MPR v2 Requirement

`diff-local` only works with MPR v2 format (Mendix >= 10.18), where documents are stored as individual files. MPR v1 projects store everything in a single SQLite database, making file-level git diff impractical.

## Workflow

### Review Before Applying

```bash
# 1. Generate MDL changes
# (AI assistant creates changes.mdl)

# 2. Review what would change
mxcli diff -p app.mpr changes.mdl

# 3. If satisfied, apply
mxcli exec changes.mdl -p app.mpr
```

### Track Changes Over Time

```bash
# After making changes, see what changed since last commit
mxcli diff-local -p app.mpr --ref HEAD

# See changes since two commits ago
mxcli diff-local -p app.mpr --ref HEAD~2
```

### Compare Branches

```bash
# What changed between main and your feature branch
mxcli diff-local -p app.mpr --ref main..feature-branch

# Feed diff into an LLM for review
mxcli diff-local -p app.mpr --ref main..feature-branch > changes.diff
```
