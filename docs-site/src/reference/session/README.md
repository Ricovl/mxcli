# Session Statements

Statements for inspecting and configuring the current REPL session.

| Statement | Description |
|-----------|-------------|
| [SET](set.md) | Set a session variable |
| [SHOW STATUS](show-status.md) | Display the current connection and session information |

## The session's language

The REPL and `mxcli -c "…"` read input without a language header as `mdl 1`.
`mxcli --mdl 0` (or `mxcli -c "…" --mdl 0`) starts them in `mdl 0`, and in the
REPL an `mdl 0;` or `mdl 1;` statement switches the session for what follows;
`describe` in the session writes the session's language. Session statements
are accepted here under every version, and the last statement of an input
needs no `;`. A script file is read by its own header; see
[Language Versions and Migration](../../language/versions.md).
