# CREATE IMAGE COLLECTION

## Synopsis

    [/** description */]
    CREATE [OR MODIFY] IMAGE COLLECTION module.name
        [EXPORT LEVEL 'Hidden' | 'Public']
        [{
            IMAGE image_name ( File: 'path' )
            IMAGE image_name ( Data: 'base64' [, Format: png | jpg | gif | svg | bmp | webp] )
            ...
        }];

## Description

Creates a new image collection in the specified module. `OR MODIFY` updates an existing collection in-place, preserving its UUID so that widget references remain valid. `OR REPLACE` is accepted as a synonym for `OR MODIFY`. Image collections bundle images (icons, logos, graphics) within a module. Images can be loaded from the filesystem during creation, with the format detected automatically from the file extension.

## Parameters

**module.name**
: The qualified name of the collection (e.g., `MyModule.AppIcons`).

**EXPORT LEVEL**
: Controls visibility from other modules. `'Hidden'` (default) restricts access to the owning module. `'Public'` makes images available to other modules.

**`/** … */`**
: Documentation text for the collection, as a doc comment before the statement. The `COMMENT '…'` clause is its deprecated alias (`MDL-DEPR100`).

**IMAGE name ( File: 'path' )**
: Loads an image from a file on disk. A relative path is resolved against the script's directory, then the current working directory. Supported formats: PNG, SVG, GIF, JPEG, BMP, WebP, taken from the file extension. A name that is not a plain identifier is written as a quoted identifier (`"logo-dark"`).

**IMAGE name ( Data: 'base64', Format: fmt )**
: The image itself, base64-encoded. This is what `DESCRIBE IMAGE COLLECTION` writes, so its output replays anywhere without the files it came from. `Format` is needed only when the bytes do not show it (PNG, JPEG, GIF, BMP and WebP signatures, and SVG markup, are recognised).

The images are the collection's children, so they are in `{ }`, each with its properties in `( )`. The older form `( IMAGE name FROM FILE 'path', … )` still parses but warns (MDL-DEPR072); `mxcli fmt --upgrade` rewrites it.

## Examples

### Empty collection

```sql
CREATE IMAGE COLLECTION MyModule.AppIcons;
```

### Public collection with description

```sql
/** Shared icons for all modules */
CREATE IMAGE COLLECTION MyModule.SharedIcons
    EXPORT LEVEL 'Public';
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
/** Company branding assets */
CREATE IMAGE COLLECTION MyModule.BrandAssets
    EXPORT LEVEL 'Public' {
    IMAGE "logo-dark" ( File: 'assets/logo-dark.png' )
    IMAGE "logo-light" ( File: 'assets/logo-light.png' )
};
```

## See Also

[DROP IMAGE COLLECTION](drop-image-collection.md), [SHOW / DESCRIBE IMAGE COLLECTION](list-describe-image-collection.md)
