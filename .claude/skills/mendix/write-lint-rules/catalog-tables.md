# Catalog-table builtins: object properties

The structs returned by `modules()`, `associations()`, `entity_event_handlers()`,
`navigation_menu_items()`, `jar_dependencies()`, `strings()`, `layouts()` and
`published_rest_operations()`. The functions themselves are listed in
[SKILL.md](SKILL.md) under "Available Query Functions". Every builtin leaves out
System and Marketplace modules, except `navigation_menu_items()`, whose rows
belong to the project rather than to a module.

### module
Returned by `modules()`.

| Property | Type | Example |
|----------|------|---------|
| `id` | string | Module UUID |
| `name` | string | `"Sales"` |
| `domain_model_documentation` | string | The module's domain model's documentation (Studio Pro's Documentation pane with nothing selected on the canvas). A Mendix module has no documentation of its own; this is the module-level text an author can write. `""` when none |

### association
Returned by `associations()`.

| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"OrderLine_Order"` |
| `qualified_name` | string | `"Sales.OrderLine_Order"` |
| `module_name` | string | `"Sales"` |
| `from_entity` | string | Qualified name of the FROM entity (the one that owns the reference): `"Sales.OrderLine"` |
| `to_entity` | string | Qualified name of the TO entity: `"Sales.Order"`; for a cross-module association, the other module's entity |
| `type` | string | `"Reference"` or `"ReferenceSet"` |
| `owner` | string | `"Default"` or `"Both"` |
| `storage_format` | string | `"Column"` or `"Table"` |
| `description` | string | Documentation text |
| `to_delete_behavior` | string | The raw Mendix value on the TO end (Mendix's `ChildDeleteBehavior`, the end MDL's `on delete` clause sets): `"DeleteMeButKeepReferences"` (the default), `"DeleteMeAndReferences"`, `"DeleteMeIfNoReferences"`. Mendix always stores a value, so *set explicitly* can only mean *not the default* |
| `from_delete_behavior` | string | The same for the FROM end (Mendix's `ParentDeleteBehavior`; Studio Pro only): `"DeleteMeButKeepReferences"`, `"DeleteMeAndReferences"`, `"DeleteMeIfNoReferences"` |
| `to_delete_error_message` | string | The message shown when a `"DeleteMeIfNoReferences"` delete on the TO end is refused; `""` otherwise |
| `from_delete_error_message` | string | The same for the FROM end |

### entity_event_handler
Returned by `entity_event_handlers()`.

| Property | Type | Example |
|----------|------|---------|
| `entity` | string | Qualified entity name: `"Sales.Order"` |
| `module_name` | string | `"Sales"` |
| `moment` | string | `"Before"` or `"After"` |
| `event` | string | `"Create"`, `"Commit"`, `"Delete"` or `"RollBack"` — Mendix's capital B; `"Rollback"` matches nothing |
| `microflow` | string | Qualified name of the handler microflow |
| `raise_error_on_false` | bool | A `"Before"` handler returning false aborts the event |
| `pass_event_object` | bool | The microflow receives the object |

### navigation_menu_item
Returned by `navigation_menu_items()`.

| Property | Type | Example |
|----------|------|---------|
| `profile` | string | Navigation profile: `"Responsive"`, `"Phone"`, `"Tablet"`, … |
| `item_path` | string | Position in the menu, dot-separated per level: `"0"`, `"0.2"` |
| `depth` | int | `0` for a top-level item |
| `caption` | string | `"Orders"` |
| `action_type` | string | `"PageAction"`, `"MicroflowAction"`, `"SignOutAction"`, `"OpenLinkAction"`, `"NoAction"`; any other action is its stored type, e.g. `"Forms$CallNanoflowClientAction"` |
| `target_page` | string | Qualified page name, for a `"PageAction"`; `""` otherwise |
| `target_microflow` | string | Qualified microflow name, for a `"MicroflowAction"`; `""` otherwise |

### jar_dependency
Returned by `jar_dependencies()`.

| Property | Type | Example |
|----------|------|---------|
| `module_name` | string | `"Sales"` |
| `group_id` | string | `"org.apache.commons"` |
| `artifact_id` | string | `"commons-lang3"` |
| `version` | string | `"3.14.0"` |
| `coordinate` | string | `"org.apache.commons:commons-lang3"` — group and artifact, without the version, so it identifies the library across versions |
| `is_included` | bool | Studio Pro's "Included" setting; False when the dependency is declared but not included |

### catalog_string
Returned by `strings(language = None)`.

| Property | Type | Example |
|----------|------|---------|
| `qualified_name` | string | The document the text belongs to: `"Sales.Order_Overview"` |
| `object_type` | string | Catalog object type, upper-case: `"PAGE"`, `"SNIPPET"`, `"LAYOUT"`, `"MICROFLOW"`, `"NANOFLOW"`, `"WORKFLOW"`, `"ENUMERATION"`, `"MENU_DOCUMENT"`, … |
| `value` | string | The text itself |
| `context` | string | What the text is. Translatable text names its stored type and property: `"Forms$Page.Title"`, `"Forms$TabPage.Caption"`, `"Enumerations$EnumerationValue.Caption"`; the rest a lower-case label: `"documentation"`, `"page_url"`, `"log_node"`, … |
| `language` | string | `"en_US"`; `""` for text that is not translatable (documentation, URLs) |
| `element_id` | string | UUID of the element carrying the text |
| `module_name` | string | `"Sales"` |

### layout
Returned by `layouts()`.

| Property | Type | Example |
|----------|------|---------|
| `name` | string | `"Atlas_Default"` |
| `qualified_name` | string | `"Atlas_Core.Atlas_Default"` |
| `module_name` | string | `"Atlas_Core"` |
| `folder` | string | Folder path within module |
| `layout_type` | string | `"Responsive"`, `"Phone"`, `"Tablet"`, `"Popup"`, `"ModalPopup"`, `"Default"`, `"Legacy"` |
| `description` | string | Documentation text |

### published_rest_operation
Returned by `published_rest_operations()`.

| Property | Type | Example |
|----------|------|---------|
| `service` | string | Qualified service name: `"Sales.OrderApi"` |
| `resource` | string | `"orders"` |
| `http_method` | string | As Mendix stores it, TitleCase — unlike `rest_operation`: `"Get"`, `"Post"`, `"Put"`, `"Patch"`, `"Delete"` |
| `path` | string | `"/{id}"` |
| `summary` | string | Operation summary |
| `microflow` | string | Qualified name of the microflow that implements the operation |
| `deprecated` | bool | Marked deprecated |
| `module_name` | string | `"Sales"` |
