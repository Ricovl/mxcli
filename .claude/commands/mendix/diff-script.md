---
description: Compare an MDL script against the project's current state
argument-hint: <script.mdl>
---

# Diff Script

Compare an MDL script against the current state of a Mendix project to see what would change.

## Commands

```bash
# Unified diff (default) - traditional +/- format
mxcli diff -p app.mpr changes.mdl

# Side-by-side comparison
mxcli diff -p app.mpr changes.mdl --format side

# Structural summary
mxcli diff -p app.mpr changes.mdl --format struct

# With color output
mxcli diff -p app.mpr changes.mdl --color

# Side-by-side with custom width
mxcli diff -p app.mpr changes.mdl --format side --width 140
```

## Output Formats

### Unified (default)

Traditional diff format showing `+` for additions and `-` for removals:

```diff
--- Entity.MyModule.Customer (current)
+++ Entity.MyModule.Customer (script)
@@ -1,5 +1,6 @@
 CREATE PERSISTENT ENTITY MyModule.Customer (
   Name: String(100) NOT NULL,
-  Email: String(200)
+  Email: String(200) NOT NULL,
+  Phone: String(20)
 );
```

### Side-by-Side (--format side)

Two-column comparison showing current vs proposed:

```
Entity.MyModule.Customer
──────────────────────────────────────────────────────────────────
Current                              │ Script
──────────────────────────────────────────────────────────────────
  Email: String(200)                 │   Email: String(200) NOT NULL,  ~
                                     │   Phone: String(20)             +
```

### Structural (--format struct)

Summary of changes by element type:

```
Entity: MyModule.Customer
  ~ Attribute Email: changed
  + Attribute Phone: String(20)

Entity: MyModule.Order
  + New
```

## What Gets Compared

`mxcli diff` runs the script with exec's own code on a scratch copy of the
project, then compares the copy with the project unit by unit. It reports
exactly the units exec would write — every document kind (domain model per
entity and association, pages, snippets, layouts, flows, security, navigation,
settings, folders) and every statement kind — each shown as its `describe`
before and after. The project itself is not changed.

A unit exec would rewrite although its description does not change is listed
on one line with the properties that change (`Modified: Page X: changed:
CanvasHeight`), or the folder it moves to.

## Summary Output

Every diff ends with a summary:

```
Summary: 2 new, 3 modified, 0 removed — exec would write 4 unit(s)
```

A script exec has already applied reports `exec would write nothing`.

A script exec would refuse (its pre-flight checks) or a statement exec would
stop at (a plain `create` of an existing document; a flow change the splice
cannot make under `mdl 1;`) is reported as `Refused: …` with exec's message.
`--no-check` and `--continue-on-error` behave as they do for exec.

## Use Cases

1. **Preview changes** before executing a script
2. **Review modifications** in a pull request
3. **Audit** what an MDL script will modify
4. **Documentation** of changes between versions

## Tips

- Use `--color` for terminal output to easily spot changes
- Use `--format struct` for a quick overview of what changes
- Use `--format side` when comparing large objects
- Run it before `exec` on a Studio Pro-authored project: a write you did not
  intend (a rewrite of a document you only meant to read back) shows up as a
  modified unit
