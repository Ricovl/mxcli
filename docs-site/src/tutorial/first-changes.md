# Your First Changes

Now that you can explore a project with `SHOW` and `DESCRIBE`, it's time to make changes. This chapter walks through the three most common modifications: creating an entity, a microflow, and a page.

## Before you start

**Always work on a copy.** mxcli writes directly to your `.mpr` file. Before making changes, either:

- Copy the `.mpr` file (and `mprcontents/` folder if it exists) to a scratch directory
- Use Git so you can revert with `git checkout`

```bash
# Make a working copy
cp app.mpr app-scratch.mpr
cp -r mprcontents/ mprcontents-scratch/   # only for MPR v2 projects
```

## The workflow

Every modification follows the same pattern:

1. **Write MDL** -- either directly on the command line with `-c`, or in a `.mdl` script file
2. **Check syntax** -- run `mxcli check` to catch errors before touching the project
3. **Execute** -- apply the changes to the `.mpr` file
4. **Validate** -- use `mxcli docker check` for a full Studio Pro-level validation
5. **Open in Studio Pro** -- confirm the result visually

```bash
# Step 1-2: Write and check syntax
mxcli check setup.mdl

# Step 3: Execute against the project
mxcli exec setup.mdl -p app.mpr

# Step 4: Full validation
mxcli docker check -p app.mpr

# Step 5: Open in Studio Pro and inspect
```

You can also skip the script file and execute commands directly:

```bash
mxcli -p app.mpr -c "CREATE PERSISTENT ENTITY MyModule.Product (Name: String(200) NOT NULL, Price: Decimal);"
```

## What we'll build

Over the next few pages, you'll create:

| What | MDL statement | Page |
|------|---------------|------|
| A `Product` entity with attributes | `CREATE PERSISTENT ENTITY` | [Creating an Entity](create-entity.md) |
| A microflow that creates products | `CREATE MICROFLOW` | [Creating a Microflow](create-microflow.md) |
| An overview page listing products | `CREATE PAGE` | [Creating a Page](create-page.md) |

The final page, [Validating with mxcli check](validation.md), covers the full validation workflow in detail.

## Quick note on module names

Every MDL statement references a **module**. In these examples we use `MyModule` -- replace it with whatever module exists in your project. You can check available modules with:

```bash
mxcli -p app.mpr -c "LIST MODULES"
```

## Idempotent scripts with OR MODIFY

If you plan to run a script more than once (common during development), use `CREATE OR MODIFY` instead of plain `CREATE`. This updates the entity if it already exists instead of failing:

```sql
CREATE OR MODIFY PERSISTENT ENTITY MyModule.Product (
    Name: String(200) NOT NULL,
    Price: Decimal,
    IsActive: Boolean DEFAULT true
);
```

The `OR MODIFY` variant is available for entities, enumerations, microflows, and pages. You'll see it used throughout the tutorial.
