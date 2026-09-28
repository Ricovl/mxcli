# LIST MODULES, LIST ENTITIES

The `SHOW` family of commands lists project elements by type. They are the fastest way to see what a project contains.

## Listing modules

Every Mendix project is organized into modules. Start by listing them:

```sql
LIST MODULES;
```

Example output:

```
MyFirstModule
Administration
Atlas_Core
System
```

By default, system and marketplace modules (like `System` and `Atlas_Core`) are included. The modules appear in the order they are defined in the project.

## Listing entities

To see all entities across all modules:

```sql
LIST ENTITIES;
```

Example output:

```
Administration.Account
MyFirstModule.Customer
MyFirstModule.Order
MyFirstModule.OrderLine
```

Each entity is shown as a **qualified name** -- the module name and entity name separated by a dot.

### Filtering by module

Most SHOW commands accept an `IN` clause to filter results to a single module:

```sql
LIST ENTITIES IN MyFirstModule;
```

```
MyFirstModule.Customer
MyFirstModule.Order
MyFirstModule.OrderLine
```

This is typically what you want when working on a specific module.

## Listing microflows

```sql
LIST MICROFLOWS;
```

```
Administration.ChangeMyPassword
MyFirstModule.ACT_Customer_Save
MyFirstModule.ACT_Order_Process
MyFirstModule.DS_Customer_GetAll
```

Filter to a module:

```sql
LIST MICROFLOWS IN MyFirstModule;
```

```
MyFirstModule.ACT_Customer_Save
MyFirstModule.ACT_Order_Process
MyFirstModule.DS_Customer_GetAll
```

## Listing pages

```sql
LIST PAGES;
```

```
Administration.Account_Overview
Administration.Login
MyFirstModule.Customer_Overview
MyFirstModule.Customer_Edit
MyFirstModule.Order_Detail
```

Filter to a module:

```sql
LIST PAGES IN MyFirstModule;
```

```
MyFirstModule.Customer_Overview
MyFirstModule.Customer_Edit
MyFirstModule.Order_Detail
```

## Other SHOW commands

The same pattern works for all major element types:

```sql
LIST ENUMERATIONS;
LIST ENUMERATIONS IN MyFirstModule;

LIST ASSOCIATIONS;
LIST ASSOCIATIONS IN MyFirstModule;

LIST WORKFLOWS;
LIST WORKFLOWS IN MyFirstModule;

LIST NANOFLOWS;
LIST NANOFLOWS IN MyFirstModule;

LIST CONSTANTS;
LIST CONSTANTS IN MyFirstModule;

LIST SNIPPETS;
LIST SNIPPETS IN MyFirstModule;
```

You can also list security-related elements:

```sql
LIST MODULE ROLES;
LIST MODULE ROLES IN MyFirstModule;

LIST USER ROLES;

LIST DEMO USERS;
```

And navigation:

```sql
LIST NAVIGATION;
```

## Using SHOW from the command line

Every SHOW command works as a CLI one-liner with `-c`:

```bash
mxcli -p app.mpr -c "LIST ENTITIES"
mxcli -p app.mpr -c "LIST MICROFLOWS IN MyFirstModule"
mxcli -p app.mpr -c "LIST PAGES"
```

This is useful for quick lookups without entering the REPL, and for piping output to other tools:

```bash
# Count entities per module
mxcli -p app.mpr -c "LIST ENTITIES" | cut -d. -f1 | sort | uniq -c

# Find all microflows with "Save" in the name
mxcli -p app.mpr -c "LIST MICROFLOWS" | grep -i save
```

## Summary of SHOW commands

| Command | Description |
|---------|-------------|
| `LIST MODULES` | List all modules |
| `LIST ENTITIES [IN Module]` | List entities |
| `LIST MICROFLOWS [IN Module]` | List microflows |
| `LIST NANOFLOWS [IN Module]` | List nanoflows |
| `LIST PAGES [IN Module]` | List pages |
| `LIST SNIPPETS [IN Module]` | List snippets |
| `LIST ENUMERATIONS [IN Module]` | List enumerations |
| `LIST ASSOCIATIONS [IN Module]` | List associations |
| `LIST CONSTANTS [IN Module]` | List constants |
| `LIST WORKFLOWS [IN Module]` | List workflows |
| `LIST BUSINESS EVENTS [IN Module]` | List business event services |
| `LIST JAVA ACTIONS [IN Module]` | List Java actions |
| `LIST MODULE ROLES [IN Module]` | List module roles |
| `LIST USER ROLES` | List user roles |
| `LIST DEMO USERS` | List demo users |
| `LIST NAVIGATION` | Show navigation profiles |

Now that you can list elements, the next step is inspecting individual elements in detail with [DESCRIBE and SEARCH](describe-search.md).
