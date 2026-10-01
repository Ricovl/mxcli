# DROP FOLDER

## Synopsis

    DROP FOLDER 'folder_path' IN module_name;

## Description

Removes an empty folder from a module. The folder must contain no documents (pages, microflows, etc.) and no sub-folders. If the folder is not empty, the command returns an error indicating how many child units remain.

Nested folder paths use the `/` separator. Only the leaf folder is deleted; parent folders are not affected.

## Parameters

**folder_path**
: The path of the folder to delete. Use `/` for nested folders (e.g., `'Orders/Processing'`).

**module_name**
: The name of the module containing the folder.

## Examples

### Drop a simple folder

```sql
DROP FOLDER 'OldPages' IN MyModule;
```

### Drop a nested folder

```sql
DROP FOLDER 'Orders/Archive' IN MyModule;
```

### Move contents out first, then drop

```sql
mdl 1;
-- Move all documents to module root
MOVE MICROFLOW MyModule.ACT_Process TO MyModule;
MOVE PAGE MyModule.ProcessDetail TO MyModule;

-- Now the folder is empty and can be dropped
DROP FOLDER 'Processing' IN MyModule;
```

## Error Conditions

| Error | Cause |
|-------|-------|
| `folder is not empty: contains N child unit(s)` | The folder still has documents or sub-folders |
| `folder not found: 'path' in Module` | No folder exists at the given path in the module |
| `module not found: Module` | The specified module does not exist |

## See Also

[CREATE FOLDER](create-folder.md), [CREATE MODULE](create-module.md), [MOVE](move.md), [MOVE FOLDER](move-folder.md)
