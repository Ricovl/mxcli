# Lexical Structure

This page describes the tokens that make up the MDL language: keywords, literals, and identifiers.

## Keywords

MDL keywords are **case-insensitive**. The following are reserved keywords:

```
ACCESS, ACTIONS, ADD, AFTER, ALL, ALTER, AND, ANNOTATION, AS, ASC,
ASCENDING, ASSOCIATION, AUTONUMBER, BATCH, BEFORE, BEGIN, BINARY,
BOOLEAN, BOTH, BUSINESS, BY, CALL, CANCEL, CAPTION, CASCADE,
CATALOG, CHANGE, CHILD, CLOSE, COLUMN, COMBOBOX, COMMIT, CONNECT,
CONFIGURATION, CONNECTOR, CONSTANT, CONSTRAINT, CONTAINER, CREATE,
CRUD, DATAGRID, DATAVIEW, DATE, DATETIME, DECLARE, DEFAULT, DELETE,
DELETE_BEHAVIOR, DELETE_BUT_KEEP_REFERENCES, DELETE_AND_REFERENCES, DEMO,
DEPTH, DESC, DESCENDING, DESCRIBE, DIFF, DISCONNECT, DROP, ELSE,
EMPTY, END, ENTITY, ENUMERATION, ERROR, EVENT, EVENTS, EXECUTE,
EXEC, EXIT, EXPORT, EXTENDS, EXTERNAL, FALSE, FOLDER, FOOTER,
FOR, FORMAT, FROM, FULL, GALLERY, GENERATE, GRANT, HEADER, HELP,
HOME, IF, IMPORT, IN, INDEX, INFO, INSERT, INTEGER, INTO, JAVA,
KEEP_REFERENCES, LABEL, LANGUAGE, LAYOUT, LAYOUTGRID, LEVEL, LIMIT,
LINK, LIST, LISTVIEW, LOCAL, LOG, LOGIN, LONG, LOOP, MANAGE, MAP,
MATRIX, MENU, MESSAGE, MICROFLOW, MICROFLOWS, MODEL, MODIFY, MODULE,
MODULES, MOVE, NANOFLOW, NANOFLOWS, NAVIGATION, NODE, NON_PERSISTENT,
NOT, NULL, OF, ON, OR, ORACLE, OVERVIEW, OWNER, PAGE, PAGES, PARENT,
PASSWORD, PERSISTENT, POSITION, POSTGRES, PRODUCTION, PROJECT,
PROTOTYPE, QUERY, QUIT, REFERENCE, REFERENCESET, REFRESH, REMOVE,
REPLACE, REPORT, RESPONSIVE, RETRIEVE, RETURN, REVOKE, ROLE, ROLES,
ROLLBACK, ROW, SAVE, SCRIPT, SEARCH, SECURITY, SELECTION, SET, SHOW,
SNIPPET, SNIPPETS, SQL, SQLSERVER, STATUS, STRING, STRUCTURE,
TABLES, TEXTBOX, TEXTAREA, THEN, TO, TRUE, TYPE, UNIQUE, UPDATE,
USER, VALIDATION, VALUE, VIEW, VIEWS, VISIBLE, WARNING, WHERE, WIDGET,
WIDGETS, WITH, WORKFLOWS, WRITE
```

Most keywords work **unquoted** as identifiers (entity names, attribute names). Only structural keywords like `CREATE`, `DELETE`, `BEGIN`, `END`, `RETURN`, `ENTITY`, and `MODULE` require quoting when used as identifiers.

> **Quoting escapes *parser* keywords only — not *platform*-reserved member names.**
> Quoting an identifier tells the MDL parser to treat it as a name rather than a token,
> so `"create"`, `"status"`, and `"type"` parse fine as attribute names. But some names
> are reserved by the Mendix *platform*, and Studio Pro rejects them **even when quoted**
> (the quotes are stripped, and the bare name is still invalid): `Type` (CE7247), the
> audit attributes `CreatedDate` / `ChangedDate` / `Owner` / `ChangedBy`, plus `ID`,
> `GUID`, `CurrentUser`, and the Java keyword list. `mxcli check --references` flags these
> as `MDL021` (CE7247) / `MDL020`. Rename them (e.g. `Type` → `ResourceType`); use the
> `AutoCreatedDate` / `AutoChangedDate` / `AutoOwner` / `AutoChangedBy` pseudo-types for
> the audit fields.

## Literals

### String Literals

String literals use single quotes:

```sql
'single quoted string'
'it''s here'            -- doubled single quote to escape
```

The only escape sequence is `''` (two single quotes) to represent a literal single quote, as in a Mendix expression. Under `mdl 1;` a backslash is an ordinary character: `'C:\temp'` is that path.

A string in a microflow or nanoflow expression is read by the same rule as any other string, and the expression stores its value the way Studio Pro does: an apostrophe doubled, a backslash and a line break as themselves. `'C:\temp'` stores `'C:\temp'`, whether the builder renders the expression or stores it as written.

A string in an XPath constraint follows the same rule, wherever the XPath is written: a retrieve's `where`, an entity access rule, a page datasource, a workflow user task's `targeting xpath`, and a navigation sync constraint. So do the Mendix expressions a statement stores as written beside them — a retrieve's `limit` and `offset`, a database query's `dynamic` expression. `describe` writes each back as stored.

A line break is written into the literal itself: a string may span lines, and the break is part of its value. This is also how a text template — the message of `log`, `show message` or `validation feedback` — holds a line break; under `mdl 1;` a template written as one literal is the template text whether or not it spans lines:

```sql
mdl 1;
create microflow Shop.LogCleanup ($Count: Integer)
begin
  log info node 'Shop' 'Deleted {1} records.
Run again tomorrow.' with ({1} = toString($Count));
end;
```

`describe` writes every string this way — a caption, an annotation, a widget property, a string in an expression: `''` is its only escape and a stored line break is a line break in the literal.

With `with ({n} = …)` parameters the literal is the template text too. Strings in a script without the header follow the older rules on [Language Versions and Migration](versions.md#strings-without-the-header).

### Numeric Literals

```sql
42          -- Integer
3.14        -- Decimal
-100        -- Negative integer
1.5e10      -- Scientific notation
```

### Boolean Literals

```sql
TRUE
FALSE
```

## Quoted Identifiers

When an identifier collides with a reserved keyword, use double quotes (ANSI SQL style) or backticks (MySQL style):

```sql
"ComboBox"."CategoryTreeVE"
`Order`.`Status`
"ComboBox".CategoryTreeVE    -- mixed quoting is allowed
```

See [Qualified Names](./qualified-names.md) for more on identifier syntax and the `Module.Name` notation.
