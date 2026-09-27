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
