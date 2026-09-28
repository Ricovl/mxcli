# The REPL

The mxcli REPL is an interactive shell for working with Mendix projects, much like `psql` is for PostgreSQL or `mysql` for MySQL. You type MDL statements, press Enter, and see results immediately.

## Starting the REPL

Launch it with or without a project:

```bash
# Start with a project already loaded
mxcli -p app.mpr

# Start without a project (you can open one later)
mxcli
```

You'll see a prompt where you can start typing MDL:

```
mxcli>
```

## Running commands

Type any MDL statement and press Enter:

```sql
LIST MODULES;
```

Results are printed as formatted tables directly in the terminal.

### Multi-line statements

MDL statements end with a semicolon (`;`). If you press Enter before typing a semicolon, mxcli knows you're still writing and waits for more input:

```sql
CREATE ENTITY MyModule.Customer (
    Name: String(200) NOT NULL,
    Email: String(200),
    IsActive: Boolean DEFAULT true
);
```

The REPL shows a continuation prompt (`.....>`) while you're in a multi-line statement. The statement executes when you type the closing `;` and press Enter.

## Getting help

Type `HELP` to see a list of available commands:

```sql
HELP;
```

This prints a categorized overview of all MDL statements -- useful when you can't remember the exact syntax for something.

## Command history

Use the **up and down arrow keys** to scroll through previous commands, just like in any other shell. This is especially handy when you're iterating on a query or tweaking a CREATE statement.

## Exiting

When you're done, type either:

```sql
EXIT;
```

or:

```sql
QUIT;
```

You can also press `Ctrl+D` to exit.

## One-off commands with `-c`

You don't always need an interactive session. The `-c` flag lets you run a single command and exit:

```bash
mxcli -p app.mpr -c "LIST ENTITIES IN MyModule"
```

This is great for quick lookups and for scripting. The output is pipe-friendly, so you can chain it with other tools:

```bash
# Count entities per module
mxcli -p app.mpr -c "LIST MODULES" | tail -n +2 | while read module; do
    echo "$module: $(mxcli -p app.mpr -c "LIST ENTITIES IN $module" | wc -l) entities"
done
```

## Running script files

For anything beyond a quick one-liner, you can put MDL statements in a `.mdl` file and execute the whole thing:

```bash
mxcli -p app.mpr -c "EXECUTE SCRIPT 'setup.mdl'"
```

Or from the REPL:

```sql
EXECUTE SCRIPT 'setup.mdl';
```

This is how you'll typically apply larger changes -- write the MDL in a file, check the syntax with `mxcli check`, then execute it.

## The TUI (Terminal UI)

If you prefer a more visual experience, mxcli also offers a graphical terminal interface:

```bash
mxcli tui -p app.mpr
```

The TUI gives you a split-pane layout with a project tree, an MDL editor, and output panels. It's built on top of the same REPL engine, so all the same commands work. This can be a nice middle ground between the bare REPL and opening Studio Pro.

## What's next

Now that you know how to install mxcli, open a project, and use the REPL, you're ready to start exploring. The next chapter walks through the commands you'll use most often: `SHOW`, `DESCRIBE`, and `SEARCH`.
