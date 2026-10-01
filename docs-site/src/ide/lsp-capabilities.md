# Capabilities

The MDL language server supports the following LSP capabilities.

## Supported Methods

| LSP Method | Feature | Notes |
|-----------|---------|-------|
| `textDocument/publishDiagnostics` | Parse and semantic error reporting | Push-based; parse on change, semantic on save |
| `textDocument/completion` | Code completion | Keywords, snippets, and project references |
| `textDocument/hover` | Hover documentation | MDL definitions for qualified names; `mxcli help <code>` on a migration warning |
| `textDocument/codeAction` | Quick fixes | The `fmt --upgrade` rewrite for a migration warning |
| `textDocument/definition` | Go-to-definition | Opens element source as virtual document |
| `textDocument/documentSymbol` | Document outline | All MDL statements in the file |
| `textDocument/foldingRange` | Code folding | Statement blocks, widget trees, comments |

## Document Synchronization

| Method | Supported |
|--------|-----------|
| `textDocument/didOpen` | Yes |
| `textDocument/didChange` | Yes (full sync) |
| `textDocument/didSave` | Yes |
| `textDocument/didClose` | Yes |

## Completion Details

Trigger characters: `.` and ` ` (space).

Completion items include:

- **Keywords** -- `CREATE`, `LIST`, `DESCRIBE`, `ALTER`, `DROP`, `GRANT`, etc. Only mdl 1
  spellings are offered: a keyword the grammar accepts only as a deprecated alias
  (`SHOW_PAGE`, `DELETE_BEHAVIOR`, `DEFINE`, ...) is not, and the listings follow
  `LIST`, not `SHOW`.
- **Keyword sequences** -- `PERSISTENT ENTITY`, `MODULE ROLE`, `USER ROLE`
- **Data types** -- `String`, `Integer`, `Boolean`, `DateTime`, `Decimal`, etc.
- **Snippets** -- Multi-line templates for entity, microflow, page creation
- **Project references** -- Module names, entity names, microflow names (requires project)

## Diagnostics Details

### Parse Diagnostics (on change)

- Syntax errors from the ANTLR4 lexer and parser
- Missing tokens, unexpected tokens, mismatched brackets
- Reported with exact line/column positions

### Language Version Diagnostics (on change)

The server reads each document's `mdl <n>;` header and reports what `mxcli check`
reports for that version, at the token or line it is about:

- **`MDL-DEPRnnn`** -- a deprecated spelling (a warning; refused from the version
  its entry names)
- **`MDL-V1-*`** -- in a script without the header, a construct whose meaning
  changes under `mdl 1;`. Under the header it means the new thing and does not warn;
  a construct mdl 1 refuses is a parse error there.
- **`MDL-LANG01`** -- the header names a version that is still a preview

Hovering over an `MDL-DEPR*` or `MDL-V1-*` warning shows `mxcli help <code>`: the old
and new form, and whether `fmt --upgrade` rewrites it. The quick fix (Ctrl+.) is that
rewrite:

- a deprecated spelling, or a change whose rewrite means the same under every version,
  is rewritten where it stands;
- any other `MDL-V1-*` change is fixed by upgrading the whole script, as
  `mxcli fmt --upgrade --header` does: its rewrite keeps the old meaning only once
  the header is there. It is offered only when that upgrade succeeds.

A use with no mechanical rewrite gets no quick fix -- it is reported, never guessed at.
With several deprecated spellings in a file, "Rewrite all ... (fmt --upgrade)" fixes
them at once. The migration codes are listed in
[Language versions and migration](../language/versions.md).

### Semantic Diagnostics (on save)

- Unresolved entity, microflow, or page references
- Invalid attribute names on known entities
- Module existence checks
- Requires a loaded Mendix project

## Not Yet Supported

The following LSP capabilities are not currently implemented:

| LSP Method | Status |
|-----------|--------|
| `textDocument/references` | Planned |
| `textDocument/rename` | Planned |
| `textDocument/formatting` | Planned |
| `textDocument/signatureHelp` | Not planned |
| `workspace/symbol` | Not planned |
