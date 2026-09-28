# Image Collections

Image collections are Mendix's way of bundling images (icons, logos, graphics) within a module. Each collection can contain multiple images in various formats (PNG, SVG, GIF, JPEG, BMP, WebP).

## Inspecting Image Collections

```sql
-- List all image collections across all modules
LIST IMAGE COLLECTIONS;

-- Filter by module
LIST IMAGE COLLECTIONS IN MyModule;

-- View full definition including embedded images
DESCRIBE IMAGE COLLECTION MyModule.AppIcons;
```

The `DESCRIBE` output includes the full `CREATE` statement. If the collection contains images, each is written into the statement as `image Name ( Data: '<base64>' )`, so the output can be copied and re-executed without the original files. In the TUI, images are rendered inline when the terminal supports it (Kitty, iTerm2, Sixel).

## CREATE IMAGE COLLECTION

```sql
[/** <description> */]
CREATE IMAGE COLLECTION <Module>.<Name>
  [EXPORT LEVEL 'Hidden'|'Public']
  [{
    IMAGE <Name> ( File: '<path>' )
    ...
  }];
```

| Option | Description | Default |
|--------|-------------|---------|
| `EXPORT LEVEL` | `'Hidden'` (internal to module) or `'Public'` (accessible from other modules) | `'Hidden'` |
| `/** … */` | Documentation for the collection, as a doc comment before the statement (`COMMENT '…'` is its deprecated alias, `MDL-DEPR100`) | (none) |
| `IMAGE Name ( File: '…' )` | Load an image from the filesystem into the collection | (none) |
| `IMAGE Name ( Data: '…' [, Format: png] )` | The image itself, base64-encoded (what `DESCRIBE` writes); `Format` only when the bytes do not show it | (none) |

The images are the collection's children, so they are in `{ }`, each with its properties in `( )`. The older form `( IMAGE Name FROM FILE '<path>', … )` still parses but warns (MDL-DEPR072); `mxcli fmt --upgrade` rewrites it.

The image format is detected automatically from the file extension. Relative paths are resolved against the script's directory, then the current working directory. Supported formats: PNG, SVG, GIF, JPEG, BMP, WebP.

### Examples

```sql
-- Minimal: empty collection
CREATE IMAGE COLLECTION MyModule.AppIcons;

-- With export level
CREATE IMAGE COLLECTION MyModule.SharedIcons EXPORT LEVEL 'Public';

-- With documentation
/** Icons for order and task status indicators */
CREATE IMAGE COLLECTION MyModule.StatusIcons;

-- With images from files
CREATE IMAGE COLLECTION MyModule.NavigationIcons {
  IMAGE home ( File: 'assets/home.png' )
  IMAGE settings ( File: 'assets/settings.svg' )
};

-- All options combined
/** Company branding assets */
CREATE IMAGE COLLECTION MyModule.BrandAssets
  EXPORT LEVEL 'Public' {
  IMAGE logo_dark ( File: 'assets/logo-dark.png' )
  IMAGE logo_light ( File: 'assets/logo-light.png' )
};
```

## DROP IMAGE COLLECTION

Remove a collection and all its embedded images:

```sql
DROP IMAGE COLLECTION MyModule.StatusIcons;
```

## Icon collections (read-only)

Distinct from image collections, **icon collections**
(`CustomIcons$CustomIconCollection`, e.g. `Atlas_Core.Atlas_Filled`) ship with the
theme/Atlas. Their icons are referenced from a widget as
`Module.Collection.IconName` — most commonly a button's `Icon:` property. They're
read-only in mxcli; use `SHOW` / `DESCRIBE` to discover valid icon names (icons
have non-obvious names — it's `add`, not `plus`):

```sql
LIST ICON COLLECTIONS;                              -- name, prefix, export level, icon count
DESCRIBE ICON COLLECTION Atlas_Core.Atlas_Filled;   -- every icon + its reference form
```

Then use one on a button:

```sql
ACTIONBUTTON btnEdit (Caption: 'Edit', Action: PAGE App.Edit,
  Icon: 'Atlas_Core.Atlas_Filled.pencil')
```

## See Also

- [MDL Quick Reference](../appendixes/quick-reference.md) -- syntax summary table
