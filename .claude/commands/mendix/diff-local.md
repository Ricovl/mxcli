---
description: Diff uncommitted mxunit changes against a git reference (MPR v2 only)
argument-hint: [--ref <rev>]
---

# Diff Local Changes

Compare local (uncommitted) changes in mxunit files against a git reference to see what has changed.

**Note:** This command only works with MPR v2 format (Mendix 10.18+) which stores units in the `mprcontents/` folder.

## Commands

```bash
# Show uncommitted changes vs HEAD (default)
mxcli diff-local -p app.mpr

# Compare against a specific commit
mxcli diff-local -p app.mpr --ref HEAD~1

# Compare against a branch
mxcli diff-local -p app.mpr --ref main

# Compare against a tag
mxcli diff-local -p app.mpr --ref v1.0.0

# With structural format
mxcli diff-local -p app.mpr --format struct

# Side-by-side comparison
mxcli diff-local -p app.mpr --format side

# With color output
mxcli diff-local -p app.mpr --color
```

## Output Formats

### Unified (default)

Traditional diff format showing `+` for additions and `-` for removals:

```diff
--- Microflow.MyModule.ProcessOrder (current)
+++ Microflow.MyModule.ProcessOrder (script)
@@ -1,5 +1,6 @@
 CREATE MICROFLOW MyModule.ProcessOrder (
   $order: MyModule.Order
+  $discount: Decimal
 )
 RETURNS Void
```

### Side-by-Side (--format side)

Two-column comparison showing git vs local:

```
Microflow.MyModule.ProcessOrder
──────────────────────────────────────────────────────────────────
Current                              │ Script
──────────────────────────────────────────────────────────────────
  $order: MyModule.Order             │   $order: MyModule.Order
                                     │   $discount: Decimal         +
```

### Structural (--format struct)

Summary of changes by element type:

```
Microflow: MyModule.ProcessOrder
  + Parameter $discount: Decimal
  ~ Lines: 2 lines added

Entity: MyModule.Customer
  + Attribute Phone: String(20)
```

## How It Works

1. **Finds changed files**: Uses `git diff --name-status` to identify modified `.mxunit` files in the `mprcontents/` folder
2. **Reads both versions**: Current version from disk, git version using `git show`
3. **Parses BSON**: Extracts the unit type and content from the binary BSON format
4. **Generates MDL**: Converts both versions to MDL for human-readable comparison
5. **Shows diff**: Outputs the differences in the selected format

## Supported Unit Types

The command generates MDL representations for:

- **Domain models**: Shows each entity's attributes, constraints and entity type, followed by its access rules as `grant` statements
- **Microflows**: Shows parameters and return type
- **Nanoflows**: Shows parameters and return type
- **Enumerations**: Shows values and captions
- **Pages**: Shows title and layout reference
- **Snippets**: Shows basic structure
- **Layouts**: Shows basic structure
- **Modules**: Shows module name
- **Project settings**: Shows the `alter settings` statements `describe settings` prints. Configuration constant values and the database password are **masked** as `<masked #1a2b3c4d>`: the fingerprint changes when the value does, so a changed value still shows as a changed line, but it is keyed per run and cannot be checked against a guess

Other unit types show a generic representation with the type name. Whenever
the two renderings of a modified unit come out the same while the stored unit
differs — a type with no renderer, or a property a renderer does not print — the
diff lists the changed fields instead, by path, type and size and never by value:

```
-- Changed fields this view does not render (values not shown):
--   DefaultValue (string, 1 → 7 chars)
--   ModuleRoles["Auditor"] (added)
```

## Summary Output

Every diff ends with a summary:

```
Summary: 2 new, 3 modified, 1 deleted
```

## Use Cases

1. **Review changes** before committing to version control
2. **Understand modifications** made by Studio Pro
3. **Audit changes** between versions
4. **Debug issues** by comparing working vs previous state

## Tips

- Use `--color` for terminal output to easily spot changes
- Use `--format struct` for a quick overview of what changed
- Use `--format side` when comparing large objects with subtle differences
- Use `--ref HEAD~5` to see what changed in the last 5 commits
- Use `--ref feature-branch` to compare against a different branch
- After restoring a `.mxunit` with `git checkout`/`git restore`, run
  `mxcli fix hashes -p app.mpr --repair`: git does not update the `.mpr`'s
  ContentsHash index, and `mx check` does not notice the stale entry

## Requirements

- MPR v2 format (Mendix 10.18+)
- Git repository with the project under version control
- The `mprcontents/` folder must be tracked by git
