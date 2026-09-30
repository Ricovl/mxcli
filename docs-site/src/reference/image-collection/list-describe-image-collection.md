# LIST / DESCRIBE IMAGE COLLECTION

## Synopsis

    LIST IMAGE COLLECTIONS;
    LIST IMAGE COLLECTIONS IN module;
    DESCRIBE IMAGE COLLECTION module.name;

## Description

`LIST IMAGE COLLECTIONS` lists all image collections in the project, optionally filtered by module. `DESCRIBE IMAGE COLLECTION` shows the full definition of a specific collection, including its images as a re-executable `CREATE` statement.

In the TUI, images are rendered inline when the terminal supports it (Kitty, iTerm2, Sixel).

## Parameters

**IN module**
: Filters the listing to a single module.

**module.name**
: The qualified name of the collection to describe (e.g., `MyModule.AppIcons`).

## Examples

### List all image collections

```sql
LIST IMAGE COLLECTIONS;
```

### Filter by module

```sql
LIST IMAGE COLLECTIONS IN MyModule;
```

### View full definition

```sql
DESCRIBE IMAGE COLLECTION MyModule.AppIcons;
```

The output is the complete `CREATE OR MODIFY` statement, each image written into it as `IMAGE name ( Data: '<base64>' )`, so it can be copied and re-executed on any machine. `DESCRIBE` writes no files.

## See Also

[CREATE IMAGE COLLECTION](create-image-collection.md), [DROP IMAGE COLLECTION](drop-image-collection.md)
