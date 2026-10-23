# LIST / DESCRIBE SETTINGS

## Synopsis

    LIST SETTINGS

    DESCRIBE SETTINGS

    DESCRIBE SETTINGS CONFIGURATION 'name'

## Description

Displays project settings. `LIST SETTINGS` provides a compact overview, one row per section. `DESCRIBE SETTINGS` outputs every section as re-executable MDL — the `ALTER SETTINGS` and `CREATE OR MODIFY CONFIGURATION` statements that recreate it. `DESCRIBE SETTINGS CONFIGURATION 'name'` outputs one configuration only, and names the ones that exist when there is none by that name. There is no form that describes another single section.

Both also show the **project ID** — the `$ID` of the `Projects$Project` root unit that every module belongs to. `LIST SETTINGS` prints it as a `Project` row; `DESCRIBE SETTINGS` prints it as a `-- Project ID: …` comment, so the output still executes. It is read-only: there is no statement that changes it. Business Events uses it as the app id, so two apps made from one model (a copied `.mpr`, a shared template) report the same project ID.

`DESCRIBE SETTINGS` emits, in order:

| Section | Emitted as |
|---------|------------|
| Runtime | `ALTER SETTINGS RUNTIME (…)` — AfterStartupMicroflow, HashAlgorithm, JavaVersion, etc.; only the properties this project's Mendix version stores. |
| Configurations | `CREATE OR MODIFY CONFIGURATION 'name' (…)` per configuration — DatabaseType, DatabaseUrl, HttpPortNumber, etc. (never `DatabasePassword`), then `ALTER SETTINGS CONSTANT … IN CONFIGURATION 'name'` per constant override. A private override — its value kept on the developer's workstation, not in the shared model — is a comment, since its value is not in the project. |
| Languages | `ALTER SETTINGS LANGUAGE ADD OR MODIFY 'code' (…)` per enabled language, then the default language. |
| Workflows | `ALTER SETTINGS WORKFLOWS (…)` and one `ADD OR MODIFY GROUP` per workflow group. |
| Web UI | `ALTER SETTINGS WEBUI (OptimizedClient: …)` — `Yes` (React client), `No` (classic client) or `MigrationMode`. |

## Examples

### Show settings overview

```sql
LIST SETTINGS;
```

### Describe all settings in MDL format

```sql
DESCRIBE SETTINGS;
```

### Describe one configuration

```sql
DESCRIBE SETTINGS CONFIGURATION 'Default';
```

### List workflow groups

```sql
LIST WORKFLOW GROUPS;
```

Lists the workflow groups from App Settings ▸ Workflows ▸ Groups with their
descriptions. It reads the project settings directly, so no catalog refresh is
needed. Groups are created with
[`ALTER SETTINGS WORKFLOWS ... GROUP`](alter-settings.md).

## See Also

[ALTER SETTINGS](alter-settings.md), [LIST CONSTANT VALUES](../query/list-constants.md)
