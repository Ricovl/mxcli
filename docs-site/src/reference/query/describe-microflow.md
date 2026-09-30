# DESCRIBE MICROFLOW / DESCRIBE NANOFLOW

## Synopsis

    DESCRIBE MICROFLOW <qualified_name>

    DESCRIBE NANOFLOW <qualified_name>

## Description

Shows the complete MDL source for a microflow or nanoflow, including parameters, return type, variable declarations, activities, control flow (IF/LOOP), error handling, and annotations. The output is round-trippable MDL that can be used directly in `CREATE MICROFLOW` or `CREATE NANOFLOW` statements.

This is useful for understanding existing logic before modifying it, or for extracting a microflow definition to use as a template.

## Parameters

*qualified_name*
: A `Module.MicroflowName` or `Module.NanoflowName` reference identifying the flow to describe. Both the module and flow name are required.

## Examples

Describe a microflow:

```sql
DESCRIBE MICROFLOW Sales.ACT_CreateOrder
```

Example output:

```sql
CREATE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
  DECLARE $Order Sales.Order;
  $Order = CREATE Sales.Order (
    OrderDate = [%CurrentDateTime%],
    Status = 'Draft'
  );
  COMMIT $Order;
  SHOW PAGE Sales.Order_Edit (Order = $Order);
  RETURN $Order;
END;
```

Describe a nanoflow:

```sql
DESCRIBE NANOFLOW MyModule.NAV_ValidateInput
```

## See Also

[LIST MICROFLOWS](list-microflows.md), [DESCRIBE PAGE](describe-page.md), [DESCRIBE ENTITY](describe-entity.md)
