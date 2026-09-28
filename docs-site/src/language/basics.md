# MDL Basics

MDL (Mendix Definition Language) is a SQL-like language for reading and modifying Mendix application projects. It provides a text-based alternative to the visual editors in Mendix Studio Pro.

## What MDL Looks Like

MDL uses familiar SQL-style syntax with Mendix-specific extensions. Here is a simple example that creates an entity with attributes:

```sql
CREATE PERSISTENT ENTITY Sales.Customer (
  CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
  Name: String(200) NOT NULL,
  Email: String(200) UNIQUE,
  IsActive: Boolean DEFAULT TRUE
)
INDEX (Name);
```

## Statement Termination

Every statement ends with a semicolon (`;`):

```sql
CREATE MODULE OrderManagement;

CREATE PERSISTENT ENTITY Sales.Order (
  OrderId: AutoNumber NOT NULL UNIQUE,
  OrderDate: DateTime NOT NULL,
)
INDEX (OrderDate DESC);
```

Under `mdl 1;` a missing `;` is an error, and so is the Oracle SQL*Plus-style `/` on its own line. A script without the header still accepts both, and `check` warns `MDL-V1-SEMI` / `MDL-V1-SLASH` for each one. At the REPL, where there is no header, a single command such as `SHOW ENTITIES` still needs no terminator.

## Trailing Commas

A trailing comma is allowed in every bracketed list — attributes, enumeration values, parameters, property lists, `{ … }` blocks — under every language version. `()` is the only way to write an empty list: `(,)` and `(a,,)` are errors.

## Keyword Case

Keywords are case-insensitive, and lowercase is canonical: `describe` writes them in lowercase and `mxcli fmt` normalizes them to it. Names are not keywords, even when they are spelled like one — a module member `User`, an attribute `Title` or a property key `Folder:` keeps its case, and so does a CamelCase value such as `ButtonStyle: Success` or a type name such as `String(200)`. Expressions, XPath, OQL and SQL are stored as written, so `fmt` leaves their text alone.

## One Spelling per Keyword

Each keyword has one spelling, and a page action uses the words a microflow uses. The older spellings still parse with the same meaning, warn with the code shown, and are rewritten by `mxcli fmt --upgrade`:

| Canonical | Deprecated | Code |
|---|---|---|
| `show page`, `save changes`, `cancel changes`, `close page`, `create object`, `delete`, `open link`, `sign out`, `complete task`, `call microflow M.F`, `call nanoflow M.F` | `show_page`, `save_changes`, `cancel_changes`, `close_page`, `create_object`, `delete_object`, `open_link`, `sign_out`, `complete_task`, `microflow M.F`, `nanoflow M.F` | `MDL-DEPR020` |
| `not null error message '…'` (also after `unique`, `required`), validation rule `error message '…'`, `on delete restrict error message '…'` | `not null error '…'`, `feedback '…'`, `error_message '…'`, `errormessage '…'` | `MDL-DEPR021` |
| `on delete cascade` / `restrict` / `set null` | `delete_behavior cascade` / `prevent` / `delete_and_references` / `delete_if_no_references` / `delete_but_keep_references` | `MDL-DEPR022` |
| `type ReferenceSet` | `type reference_set` | `MDL-DEPR023` |
| `returns nothing` (REST call) | `returns none` | `MDL-DEPR024` |

## One Verb per Job

`list` enumerates, `describe` shows one thing, and an `alter` adds and drops its children. The older verbs still parse with the same meaning and warn with the code shown; `mxcli fmt --upgrade` rewrites them, except where noted:

| Canonical | Deprecated | Code |
|---|---|---|
| `describe page M.P`, `describe app security`, `describe security matrix [in M]`, `describe structure …`, `describe context of X` | `show` (or `list`) with the same words; `show project security` | `MDL-DEPR090` |
| `describe entity X`, `describe association X`, `describe navigation`, `describe settings` | `show entity X`, `show association X`, `show navigation [menu]`, `show settings` — these print a summary where `describe` prints the definition as MDL, so `fmt --upgrade` reports them and leaves them in place | `MDL-DEPR090` |
| `alter user role R drop module roles (…)` | `… remove module roles (…)` | `MDL-DEPR091` |
| `alter settings language drop '…'`, `alter settings workflows drop group '…'` | `remove` | `MDL-DEPR092` |
| `alter entity E add\|rename\|modify\|drop attribute …` | `… column …` | `MDL-DEPR093` |
| `call rest service get '…' …` | `rest call get '…' …` | `MDL-DEPR094` |
| `describe widget type combobox` | `describe widget combobox` | `MDL-DEPR095` |
| `create fragment F as { … }` | `define fragment F as { … }` | `MDL-DEPR096` |

## Language Version Header

A script may start with a header that names the MDL language version it is written in:

```sql
mdl 1;

create persistent entity Sales.Customer (
  Name: String(200)
);
```

- **No header** means `mdl 0`, the current (alpha) language. When a construct means something different under `mdl 1`, a headerless script keeps the old meaning and `check`/`exec` warn about it. A script's meaning never depends on which mxcli release runs it.
- **`mdl 1;`** selects the beta language. Until beta it is a **preview**: it parses, but warns `preview: may still change` (`MDL-LANG01`), and `describe` and `fmt` do not emit it.
- The header must be the **first** statement. A version this mxcli does not know is refused.
- It is independent of the Mendix version your project targets.

What `mdl 1` makes strict (each is a warning without the header, with the code shown):

| Under `mdl 1;` | Without the header |
|---|---|
| A statement without `;` is an error. | Accepted; `MDL-V1-SEMI`. |
| A `/` terminator line is an error. | Accepted; `MDL-V1-SLASH`. |
| `''` is the only string escape; a backslash is an ordinary character, so `'C:\temp'` is that path. | `\n`, `\r`, `\t`, `\\` and `\'` are escapes; `MDL-V1-ESCAPE` for each literal whose value would change. |
| In a REST client, published REST service, business event service, model, knowledge base, consumed MCP service or agent, an unknown property key is an error that names the key it most likely meant, and so is a value its key does not take (`Response: json from $X`). | The property is ignored, or read by its shape as before; `MDL-V1-PROP` / `MDL-V1-PROPVALUE`. |
| A `while` loop is `while <condition> begin … end while;`; leaving out `begin`, or the `while` after `end`, is an error. | Accepted; `MDL-V1-WHILE`. |
| A session command — `connect`, `disconnect`, `use`, `set format = …`, `status`, `check`, `build`, `lint`, `debug`, `execute script`, `execute runtime`, `help`, `introspect api` — is an error in a script. Type it at the REPL, or use the command-line flag (`mxcli exec script.mdl -p app.mpr --json`). The REPL keeps accepting them. | Runs as before; `MDL-V1-SESSION`. |
| A statement that starts with an unknown word (`craete module Foo;`, a stray `PRIVATE;`) is an error at the word: `unknown statement 'craete' — did you mean 'create'?`. | Does nothing, as before; `MDL-V1-UNKNOWN`. |

### Upgrading a script: `mxcli fmt --upgrade`

`mxcli fmt --upgrade` rewrites every deprecated spelling (the `MDL-DEPRnnn` warnings) to its canonical form — `create or replace` becomes `create or modify`, `show entities` becomes `list entities`, `on error { … }` becomes `on error begin … end error` — and changes nothing else: comments, layout and keyword case are kept. A deprecated use with no mechanical rewrite is reported and left in place.

```bash
mxcli fmt --upgrade script.mdl            # print the upgraded script
mxcli fmt --upgrade -w script.mdl         # upgrade in place
mxcli fmt --upgrade --header -w script.mdl  # also add `mdl 1;`
```

`--header` adds `mdl 1;` after rewriting every construct whose meaning the header would change, so the script keeps doing what it did:

| Code | Rewrite |
|---|---|
| `MDL-V1-SEMI` | adds the missing `;` |
| `MDL-V1-SLASH` | deletes the `/` line |
| `MDL-V1-ESCAPE` | writes the string's value with `''` as the only escape (`'it\'s\t'` becomes `'it''s` + a tab + `'`) |
| `MDL-V1-LIMIT1` | `retrieve … limit 1` (one object) becomes `retrieve … first` |
| `MDL-V1-SET` | `$x = …` becomes `set $x = …` |
| `MDL-V1-LIST`, `MDL-DEPR003`, `MDL-DEPR004` | a list operation or aggregate call becomes its statement form (`$x = filter($L, …)` → `$x = filter $L where …`); `find`/`contains` on a declared String keeps the call and gains `set` |
| `MDL-V1-REPLACE02` | `create or replace user role` / `demo user` becomes a plain `create` |
| `MDL-V1-WHILE` | inserts the missing `begin` after a `while` condition and `while` after its `end` |

A construct with no mechanical rewrite is reported with the reason, and `fmt` refuses to add the header rather than change the script's meaning: an unknown or mis-shaped property (`MDL-V1-PROP`, `MDL-V1-PROPVALUE`), `create or replace view entity` (`MDL-V1-REPLACE01`), a session command in a script (`MDL-V1-SESSION`: move it to the command line or the REPL), a statement that starts with an unknown word (`MDL-V1-UNKNOWN`: correct the keyword or delete it), a nested list operation such as `count(filter(…))`, `find`/`contains` on a variable whose type the script does not state, and an escaped line break (`\n`) inside an expression. While `mdl 1` is a preview, the header is added only when asked. Running `fmt --upgrade` on its own output changes nothing.

The design is in [ADR-0011](https://github.com/mendixlabs/mxcli/blob/main/docs/13-decisions/0011-mdl-language-versioning.md); `mxcli syntax language-header` has the details.

## Case Insensitivity

All MDL **keywords** are case-insensitive. The following are equivalent:

```sql
CREATE PERSISTENT ENTITY Sales.Customer ( ... );
create persistent entity Sales.Customer ( ... );
Create Persistent Entity Sales.Customer ( ... );
```

Identifiers (module names, entity names, attribute names) are case-sensitive and must match the Mendix model exactly.

## Statement Categories

MDL statements fall into several categories:

| Category | Examples |
|----------|----------|
| **Query** | `SHOW ENTITIES`, `DESCRIBE ENTITY`, `SEARCH` |
| **Domain Model** | `CREATE ENTITY`, `CREATE ASSOCIATION`, `ALTER ENTITY` |
| **Enumerations** | `CREATE ENUMERATION`, `ALTER ENUMERATION` |
| **Microflows** | `CREATE MICROFLOW`, `DROP MICROFLOW` |
| **Pages** | `CREATE PAGE`, `ALTER PAGE`, `CREATE SNIPPET` |
| **Security** | `GRANT`, `REVOKE`, `CREATE USER ROLE` |
| **Navigation** | `CREATE OR REPLACE NAVIGATION` |
| **Connection** | `CONNECT LOCAL`, `DISCONNECT`, `STATUS` |

## Further Reading

- [Lexical Structure](./lexical-structure.md) -- keywords, literals, and tokens
- [Qualified Names](./qualified-names.md) -- how elements are referenced
- [Comments](./comments.md) -- comment syntax
- [Script Files](./script-files.md) -- running MDL from files
