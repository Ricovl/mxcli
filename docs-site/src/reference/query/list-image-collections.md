# LIST IMAGE COLLECTIONS

## Synopsis

    LIST IMAGE COLLECTIONS;
    LIST IMAGE COLLECTIONS IN module;

## Description

Lists all image collections in the project. Use `IN module` to filter to a specific module. For full details including embedded images, use `DESCRIBE IMAGE COLLECTION`.

## Parameters

**IN module**
: Optional. Restricts the listing to collections in the specified module.

## Examples

### List all collections

```sql
LIST IMAGE COLLECTIONS;
```

### Filter by module

```sql
LIST IMAGE COLLECTIONS IN MyModule;
```

### Describe a specific collection

```sql
DESCRIBE IMAGE COLLECTION MyModule.AppIcons;
```

## See Also

[DESCRIBE IMAGE COLLECTION](../image-collection/list-describe-image-collection.md), [CREATE IMAGE COLLECTION](../image-collection/create-image-collection.md), [LIST MODULES](list-modules.md)
