# SHOW STATUS

## Synopsis

    STATUS

## Description

Displays the current session status including the connection state, the open project path, the Mendix version of the project, the MPR format version, and the values of any session variables. This is useful for confirming which project is loaded and verifying session configuration.

## Parameters

This statement takes no parameters.

## Examples

### Check session status

```sql
STATUS;
```

Sample output:

```
Status: Connected
Project: /Users/dev/projects/MyApp/MyApp.mpr
Version: 10.6.0
Format:  v2
Modules: 5
```

### Verify connection before running commands

```sql
STATUS;
LIST MODULES;
```

## See Also

[OPEN PROJECT](../connection/open-project.md), [CLOSE PROJECT](../connection/close-project.md), [SET](set.md)
