# OPEN PROJECT

## Synopsis

    CONNECT LOCAL 'path/to/app.mpr'

## Description

Opens a Mendix project file (`.mpr`) for reading and modification. The path can be absolute or relative to the current working directory. Once connected, all query and mutation statements operate against this project.

MDL supports both MPR v1 (single `.mpr` SQLite file, Mendix < 10.18) and MPR v2 (`.mpr` metadata + `mprcontents/` folder, Mendix >= 10.18). The format is detected automatically.

Only one project can be open at a time. If a project is already open, close it with `DISCONNECT` before opening another.

## Parameters

**path**
: Path to the `.mpr` file. Can be an absolute path or a path relative to the current working directory. The path must be enclosed in single quotes.

## Examples

### Open a project with an absolute path

```sql
CONNECT LOCAL '/Users/dev/projects/MyApp/MyApp.mpr';
```

### Open a project with a relative path

```sql
CONNECT LOCAL './mx-test-projects/test1-go-app/test1-go.mpr';
```

### Open via the CLI flag

Instead of using `CONNECT LOCAL` inside the REPL, you can specify the project path when launching the CLI:

```sql
-- These are shell commands, not MDL:
-- mxcli -p /path/to/app.mpr
-- mxcli -p app.mpr -c "LIST ENTITIES"
```

## See Also

[CLOSE PROJECT](close-project.md), [SHOW STATUS](../session/show-status.md)
