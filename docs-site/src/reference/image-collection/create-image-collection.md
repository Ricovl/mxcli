# CREATE IMAGE COLLECTION

## Synopsis

    CREATE [OR MODIFY] IMAGE COLLECTION module.name
        [EXPORT LEVEL 'Hidden' | 'Public']
        [COMMENT 'description']
        [{
            IMAGE image_name ( File: 'path' )
            ...
        }];

## Description

Creates a new image collection in the specified module. `OR MODIFY` updates an existing collection in-place, preserving its UUID so that widget references remain valid. `OR REPLACE` is accepted as a synonym for `OR MODIFY`. Image collections bundle images (icons, logos, graphics) within a module. Images can be loaded from the filesystem during creation, with the format detected automatically from the file extension.

## Parameters

**module.name**
: The qualified name of the collection (e.g., `MyModule.AppIcons`).

**EXPORT LEVEL**
: Controls visibility from other modules. `'Hidden'` (default) restricts access to the owning module. `'Public'` makes images available to other modules.

**COMMENT**
: Documentation text for the collection.

**IMAGE name ( File: 'path' )**
: Loads an image from a file on disk. The path is relative to the current working directory. Supported formats: PNG, SVG, GIF, JPEG, BMP, WebP. A name that is not a plain identifier is written as a quoted identifier (`"logo-dark"`).

The images are the collection's children, so they are in `{ }`, each with its properties in `( )`. The older form `( IMAGE name FROM FILE 'path', … )` still parses but warns (MDL-DEPR072); `mxcli fmt --upgrade` rewrites it.

## Examples

### Empty collection

```sql
CREATE IMAGE COLLECTION MyModule.AppIcons;
```

### Public collection with description

```sql
CREATE IMAGE COLLECTION MyModule.SharedIcons
    EXPORT LEVEL 'Public'
    COMMENT 'Shared icons for all modules';
```

### Collection with images

```sql
CREATE IMAGE COLLECTION MyModule.NavigationIcons {
    IMAGE home ( File: 'assets/home.png' )
    IMAGE settings ( File: 'assets/settings.svg' )
    IMAGE profile ( File: 'assets/profile.png' )
};
```

### All options combined

```sql
CREATE IMAGE COLLECTION MyModule.BrandAssets
    EXPORT LEVEL 'Public'
    COMMENT 'Company branding assets' {
    IMAGE "logo-dark" ( File: 'assets/logo-dark.png' )
    IMAGE "logo-light" ( File: 'assets/logo-light.png' )
};
```

## See Also

[DROP IMAGE COLLECTION](drop-image-collection.md), [SHOW / DESCRIBE IMAGE COLLECTION](show-describe-image-collection.md)
