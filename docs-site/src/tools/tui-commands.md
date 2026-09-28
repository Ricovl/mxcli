# Command Palette

Press `:` to open the command palette at the bottom of the TUI. The command bar provides VS Code-style command execution with tab completion and fuzzy matching.

## Usage

1. Press `:` to activate the command bar.
2. Start typing a command name. Tab completion suggests matching commands.
3. Press `Enter` to execute or `Esc` to cancel.

## Available Commands

| Command | Description |
|---------|-------------|
| `:check` | Validate the project with `mx check` |
| `:run` | Execute the current MDL file |
| `:callers` | Show callers of the selected element |
| `:callees` | Show callees of the selected element |
| `:context` | Describe context of the selected element |
| `:impact` | Show impact analysis of the selected element |
| `:refs` | Show references to the selected element |
| `:diagram` | Open a diagram of the selected element in the system browser |
| `:search <keyword>` | Perform full-text search across the project |

## Tab Completion

The command bar supports multi-level tab completion:

1. **Level 1 -- Command name**: Type the first few characters and press `Tab` to complete. For example, `cal` + `Tab` completes to `callers`.
2. **Level 2 -- Subcommand**: Some commands have subcommands. Tab cycles through available options.
3. **Level 3 -- Qualified name**: Commands that operate on elements (callers, callees, refs, impact, context) complete with qualified names from the project tree. For example, `:callers MyMod` + `Tab` expands to `:callers MyModule.ACT_CreateCustomer`.

Fuzzy matching allows partial input. Typing `:cal` matches `:callers` and `:callees`.

## Code Navigation Commands

The `:callers`, `:callees`, `:refs`, `:impact`, and `:context` commands display their results in a fullscreen overlay with syntax highlighting. These commands require a prior `REFRESH CATALOG FULL` to populate the cross-reference index.

When the currently selected element in the browser is a microflow, page, or other navigable document, these commands automatically use it as the target. You can also provide an explicit qualified name:

```
:callers MyModule.ACT_CreateCustomer
:impact MyModule.Customer
```

## Search

The `:search` command takes a keyword argument and performs full-text search across all project strings:

```
:search validation
:search Customer
```

Results appear in a fullscreen overlay listing matching elements with their type and location.

## Diagram

The `:diagram` command opens a visual diagram of the selected element in the system browser. It uses `xdg-open` on Linux or `open` on macOS to launch the default browser. This is useful for viewing domain model relationships or microflow graphs in a visual format.
