# LIST NAVIGATION

## Synopsis

```sql
LIST NAVIGATION
LIST NAVIGATION MENU [ profile ]
LIST NAVIGATION HOMES
DESCRIBE NAVIGATION [ profile ]
```

## Description

Displays navigation configuration for the current project.

### LIST NAVIGATION

Displays a summary of all configured navigation profiles, showing the profile type and whether a home page, login page, and menu are configured.

### LIST NAVIGATION MENU

Displays the menu tree for one or all navigation profiles. When a profile is specified, only that profile's menu is shown. Without a profile, all profiles' menus are displayed.

The menu is listed as a table, one row per item in the order DESCRIBE prints them: the profile, the caption indented by depth, the action's target, and the **Address** — the shortest unique caption path that [ALTER NAVIGATION](alter-navigation.md) takes to reach the item (`'Admin' > 'Users'`, or `'Home'@2` where captions repeat). A native profile's bottom bar has no address, because ALTER NAVIGATION does not edit it.

### LIST NAVIGATION HOMES

Displays home page assignments across all navigation profiles, including role-specific overrides.

### DESCRIBE NAVIGATION

Outputs the full MDL representation of one or all navigation profiles. The output is round-trippable -- it can be re-executed with `CREATE OR REPLACE NAVIGATION` to recreate the profile.

## Parameters

`profile`
:   Optional. One of: `Responsive`, `Tablet`, `Phone`, `NativePhone`. When omitted, all profiles are shown.

## Examples

Show a summary of all profiles:

```sql
LIST NAVIGATION;
```

Show the menu tree for the responsive profile:

```sql
LIST NAVIGATION MENU Responsive;
```

Show all home page assignments:

```sql
LIST NAVIGATION HOMES;
```

Export the responsive profile as MDL:

```sql
DESCRIBE NAVIGATION Responsive;
```

## See Also

[CREATE NAVIGATION](create-navigation.md), [ALTER NAVIGATION](alter-navigation.md)
