# CREATE NANOFLOW

## Synopsis

```sql
CREATE [ OR REPLACE ] NANOFLOW module.Name
    [ ( DECLARE $param : type [, ...] ) ]
    [ RETURN type ]
    [ FOLDER 'path' ]
BEGIN
    statements
END
```

## Description

Creates a new nanoflow in the specified module. Nanoflows share the same syntax as microflows but execute on the client (web browser or native mobile app) rather than on the server. This makes them suitable for offline-capable logic, instant UI feedback, and reducing server round-trips.

If `OR REPLACE` is specified and a nanoflow with the same qualified name already exists, it is replaced.

### Restricted Activity Set

Because nanoflows run on the client, several server-side activities are **not available**:

- No direct database retrieval with XPath (`RETRIEVE ... FROM Entity WHERE ...` over database)
- No `COMMIT` with database transactions
- No `LOG` statements (server log is inaccessible)
- No `EXECUTE DATABASE QUERY`
- No server-side Java action calls

Available activities in nanoflows include:

- `CREATE`, `CHANGE`, `DELETE` (on client-side objects)
- `COMMIT` (triggers synchronization when online)
- `RETRIEVE` by association
- `CALL NANOFLOW` (call other nanoflows)
- `CALL MICROFLOW` (triggers a server round-trip)
- `SHOW PAGE`, `CLOSE PAGE`
- `VALIDATION FEEDBACK`
- `IF`, `LOOP`, `WHILE`, `RETURN`
- Variable declaration and assignment

### Parameters, Return Type, and Folder

These work identically to [CREATE MICROFLOW](create-microflow.md). See that page for full details on types, annotations, and folder placement.

## Parameters

`module.Name`
:   The qualified name of the nanoflow (`Module.NanoflowName`). The module must already exist.

`$param : type`
:   A parameter declaration. Same types as microflow parameters: primitives, entities, lists, enumerations.

`RETURN type`
:   The return type of the nanoflow.

`FOLDER 'path'`
:   Optional folder path within the module.

## Examples

Nanoflow that validates an object on the client:

```sql
CREATE NANOFLOW Sales.NAV_ValidateOrder
    (DECLARE $Order: Sales.Order)
    RETURN Boolean
BEGIN
    IF $Order/CustomerName = empty THEN
        VALIDATION FEEDBACK $Order/CustomerName MESSAGE 'Customer name is required';
        RETURN false;
    END IF;
    IF $Order/Amount <= 0 THEN
        VALIDATION FEEDBACK $Order/Amount MESSAGE 'Amount must be positive';
        RETURN false;
    END IF;
    RETURN true;
END;
```

Nanoflow that toggles a UI state:

```sql
CREATE NANOFLOW MyModule.NAV_ToggleDetail
    (DECLARE $Helper: MyModule.UIHelper)
BEGIN
    IF $Helper/ShowDetail = true THEN
        CHANGE $Helper (ShowDetail = false);
    ELSE
        CHANGE $Helper (ShowDetail = true);
    END IF;
END;
```

Nanoflow calling a microflow for server-side work:

```sql
CREATE NANOFLOW Sales.NAV_SubmitOrder
    (DECLARE $Order: Sales.Order)
BEGIN
    CHANGE $Order (Status = 'Submitted');
    $Result = CALL MICROFLOW Sales.ACT_ProcessOrder (Order = $Order);
    SHOW PAGE Sales.Order_Confirmation (Order = $Order);
END;
```

## See Also

[CREATE MICROFLOW](create-microflow.md), [DROP MICROFLOW](drop-microflow.md), [GRANT EXECUTE ON NANOFLOW](/reference/security/grant.md)
