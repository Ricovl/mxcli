# Opening Your First Project

mxcli works with Mendix project files -- the `.mpr` files that Mendix Studio Pro creates. Let's open one.

> **Back up first.** mxcli is alpha software and can corrupt project files. Before you start, either make a copy of your `.mpr` file or make sure the project is under version control (Git). This isn't just a disclaimer -- take it seriously.

## The `-p` flag

The most common way to point mxcli at a project is with the `-p` flag:

```bash
mxcli -p /path/to/app.mpr -c "LIST MODULES"
```

This opens the project in read-only mode, runs the command, and exits. The `-p` flag works with all mxcli subcommands.

`-c` takes several statements separated by `;`. Like `mxcli exec`, it stops at the first statement that fails, prints `statement 2 of 3: …` and how many later statements were not run, and exits non-zero; add `--continue-on-error` to run every statement and report each failure. An empty `-c ""` is an error: it never opens the interactive REPL, so a script that generates an empty command list cannot hang waiting on input.

## Opening in the REPL

You can also open a project from inside the interactive REPL:

```bash
# Start the REPL, then open a project
mxcli
```

```sql
OPEN PROJECT '/path/to/app.mpr';
LIST MODULES;
```

Or pass `-p` when launching the REPL so it opens the project immediately:

```bash
mxcli -p /path/to/app.mpr
```

```sql
-- Project is already open, start working
LIST MODULES;
```

When you provide `-p`, the project stays open for the duration of your REPL session. You don't need to pass it again for each command.

## Read-only vs read-write

By default, mxcli opens projects in **read-only** mode. This is safe for exploration -- you can browse modules, describe entities, search through the project, and run catalog queries without risk.

When you execute a command that modifies the project (like `CREATE ENTITY`), mxcli automatically upgrades to read-write mode. You'll see a confirmation message when this happens.

## MPR format auto-detection

Mendix projects come in two formats:

- **v1**: A single `.mpr` SQLite database file (Mendix versions before 10.18)
- **v2**: An `.mpr` metadata file plus an `mprcontents/` folder with individual documents (Mendix 10.18 and later)

You don't need to worry about which format your project uses. mxcli detects the format automatically and handles both transparently. Just point it at the `.mpr` file either way.

## Working in a Dev Container

If you're using the Dev Container setup from the previous page, your project is already mounted in the container. The typical path is:

```bash
mxcli -p app.mpr
```

The Dev Container sets the working directory to your project root, so relative paths work naturally.

## Quick sanity check

Once you have a project open, try listing the modules:

```bash
mxcli -p app.mpr -c "LIST MODULES"
```

You should see a table of module names. If you see your project's modules listed, everything is working and you're ready to explore.

Next up: the REPL, where the real fun starts.
