# String Template Syntax Specification

This document describes the unified syntax for string templates with parameters in MDL (Mendix Definition Language). String templates allow embedding dynamic values into text using placeholders like `{1}`, `{2}`, etc.

## Overview

String templates are used in both microflows and pages to create dynamic text:
- **Microflows**: LOG statements, ShowMessage actions
- **Pages**: DynamicText content, ActionButton captions

Although the underlying Mendix metamodel uses different types (`microflows$stringtemplate` vs `Forms$ClientTemplate`), MDL provides a **unified syntax** using the `with` clause.

## Basic Syntax

```sql
'Template text with {1} and {2}' with ({1} = expr1, {2} = expr2)
```

### Components

| Component | Description | Example |
|-----------|-------------|---------|
| Template text | String with `{n}` placeholders | `'Order {1} has {2} items'` |
| `with` clause | Maps placeholders to values | `with ({1} = $OrderNumber, {2} = $ItemCount)` |
| Placeholder | `{n}` where n is 1-based index | `{1}`, `{2}`, `{3}` |
| Expression | Value for the placeholder | `$Variable`, `'literal'`, `$widget.Attribute` |

## Expression Types

### Variables and Parameters

Use `$` prefix for variables and parameters:

```sql
-- Microflow parameter
'Processing order {1}' with ({1} = $OrderNumber)

-- Page parameter
'Welcome {1}' with ({1} = $Customer/Name)
```

### String Literals

Use single quotes for literal values:

```sql
'Status: {1}' with ({1} = 'Active')
```

### Data Source Attribute References (Pages Only)

In pages, widgets are nested within data containers (DataView, ListView, Gallery). To reference attributes from a specific data source, use the **widget name** as a qualifier:

```sql
$WidgetName.AttributeName
```

This explicitly identifies which data source provides the attribute value.

#### Example: Nested Data Sources

```sql
create page Sales.OrderDetailPage ($Order: Sales.Order)
begin
  -- DataView named 'dvOrder' bound to page parameter
  dataview dvOrder datasource $Order
  begin
    -- $dvOrder.OrderNumber refers to the dvOrder's context (Sales.Order)
    dynamictext txtOrderNum (
      content 'Order #{1}' with ({1} = $dvOrder.OrderNumber)
    );

    -- Nested ListView named 'lvItems'
    listview lvItems datasource $dvOrder/Sales.Order_OrderItem/Sales.OrderItem
    begin
      -- Can reference BOTH parent (dvOrder) and current (lvItems) data sources
      dynamictext txtItem (
        content 'Order {1}: {2}x {3} @ ${4}'
        with (
          {1} = $dvOrder.OrderNumber,   -- from parent DataView (Sales.Order)
          {2} = $lvItems.Quantity,       -- from current ListView (Sales.OrderItem)
          {3} = $lvItems.ProductName,    -- from current ListView (Sales.OrderItem)
          {4} = $lvItems.UnitPrice       -- from current ListView (Sales.OrderItem)
        )
      );

      -- Further nested DataView for product details
      dataview dvProduct datasource $lvItems/Sales.OrderItem_Product/Sales.Product
      begin
        -- Three levels deep - can reference all in-scope data sources
        dynamictext txtProductInfo (
          content 'Order {1} | Qty: {2} | Product: {3} (SKU: {4})'
          with (
            {1} = $dvOrder.OrderNumber,   -- grandparent (Sales.Order)
            {2} = $lvItems.Quantity,       -- parent (Sales.OrderItem)
            {3} = $dvProduct.Name,         -- current (Sales.Product)
            {4} = $dvProduct.SKU           -- current (Sales.Product)
          )
        );
      end;
    end;
  end;
end;
```

### Expressions

Any valid Mendix expression can be used:

```sql
'Total: {1}' with ({1} = toString($Total * 1.21))
'Items: {1}' with ({1} = toString(length($ItemList)))
```

## Usage in Microflows

### LOG Statement

```sql
-- Simple template
log info node 'OrderService' 'Processing order: {1}' with ({1} = $OrderNumber);

-- Multiple parameters
log info node 'OrderService' 'Order {1} for {2} totaling {3}' with (
  {1} = $OrderNumber,
  {2} = $CustomerName,
  {3} = toString($TotalAmount)
);

-- Without template (simple concatenation still works)
log info node 'OrderService' 'Processing order: ' + $OrderNumber;
```

### ShowMessage Action (Future)

```sql
show message info 'Order {1} created successfully' with ({1} = $OrderNumber);
```

## Usage in Pages

### DynamicText Widget

```sql
-- Simple template with literal
dynamictext txtWelcome (
  content 'Welcome {1}!' with ({1} = 'User'),
  rendermode 'H3'
);

-- Template with data source attribute
dynamictext txtOrderInfo (
  content 'Order #{1} - {2}' with (
    {1} = $dvOrder.OrderNumber,
    {2} = $dvOrder.Status
  ),
  rendermode 'Paragraph'
);
```

### ActionButton Caption

```sql
-- Button with dynamic caption
actionbutton btnConfirm 'Confirm Order #{1}'
  with ({1} = $dvOrder.OrderNumber)
  action call_microflow 'Sales.ConfirmOrder';

-- Multiple placeholders
actionbutton btnProcess 'Process {1} items for ${2}'
  with ({1} = $dvOrder.ItemCount, {2} = $dvOrder.TotalAmount)
  action save_changes
  style primary;
```

## Mendix Internal Mapping

### Microflows: `microflows$stringtemplate`

The MDL syntax maps to the Mendix internal structure:

```
MDL:
  'Order {1} for {2}' with ({1} = $OrderNumber, {2} = $CustomerName)

BSON:
  {
    "$type": "microflows$stringtemplate",
    "text": "'Order {1} for {2}'",
    "parameters": [
      {"$type": "microflows$TemplateParameter", "expression": "$OrderNumber"},
      {"$type": "microflows$TemplateParameter", "expression": "$CustomerName"}
    ]
  }
```

### Pages: `Forms$ClientTemplate`

When referencing a page parameter's attribute (e.g., `$Product.Name` where `$Product` is a page parameter):

```
MDL:
  content: '$Product.Name'
  -- or explicit --
  content: 'Product: {1}', contentparams: ({1} = $Product.Name)

BSON:
  {
    "$type": "Forms$ClientTemplate",
    "template": {"$type": "Texts$text", "Items": [{"text": "{1}"}]},
    "parameters": [
      {
        "$type": "Forms$ClientTemplateParameter",
        "AttributeRef": {"attribute": "Sales.Product.Name"},
        "expression": "",
        "SourceVariable": {
          "$type": "Forms$PageVariable",
          "PageParameter": "Product",
          "UseAllPages": false,
          "widget": ""
        }
      }
    ]
  }
```

**Important**: When `SourceVariable` is set, it indicates the parameter binding. The `AttributeRef` still contains the full entity path for type resolution, but the `SourceVariable.PageParameter` preserves which page parameter is being referenced. This allows distinguishing between multiple parameters of the same entity type.

When referencing a widget's data source attribute (e.g., `$dvOrder.OrderNumber` where `dvOrder` is a DataView):

```
MDL:
  'Order #{1}' with ({1} = $dvOrder.OrderNumber)

BSON:
  {
    "$type": "Forms$ClientTemplate",
    "template": {"$type": "Texts$text", "Items": [{"text": "Order #{1}"}]},
    "parameters": [
      {
        "$type": "Forms$ClientTemplateParameter",
        "AttributeRef": {"attribute": "Sales.Order.OrderNumber"},
        "expression": "",
        "SourceVariable": {
          "$type": "Forms$PageVariable",
          "PageParameter": "",
          "UseAllPages": false,
          "widget": "dvOrder"
        }
      }
    ]
  }
```

When using a simple expression (not a data source attribute):

```
MDL:
  'Hello {1}' with ({1} = 'World')

BSON:
  {
    "$type": "Forms$ClientTemplate",
    "parameters": [
      {
        "$type": "Forms$ClientTemplateParameter",
        "AttributeRef": null,
        "expression": "'World'"
      }
    ]
  }
```

## Syntax Reference

### Complete Grammar

```
templateString
    : STRING_LITERAL (with LPAREN templateParamList RPAREN)?
    ;

templateParamList
    : templateParam (COMMA templateParam)*
    ;

templateParam
    : LBRACE NUMBER RBRACE equals templateValue
    ;

templateValue
    : expression                    // Any Mendix expression
    | dataSourceAttributeRef        // $WidgetName.Attribute (pages only)
    ;

dataSourceAttributeRef
    : VARIABLE DOT IDENTIFIER       // e.g., $dvOrder.OrderNumber
    ;
```

### Summary Table

| Context | Syntax | Example |
|---------|--------|---------|
| Microflow variable | `$VarName` | `$OrderNumber` |
| Microflow attribute | `$Var/attr` | `$Order/OrderNumber` |
| Page parameter | `$ParamName` | `$Customer` |
| Page parameter attribute | `$ParamName.Attr` | `$Product.Name` |
| Page data source attribute | `$WidgetName.Attr` | `$dvOrder.OrderNumber` |
| String literal | `'text'` | `'Hello'` |
| Expression | any expression | `toString($Total)` |

## Migration from PARAMETERS Syntax

The previous `parameters [...]` syntax is deprecated. Migrate as follows:

```sql
-- Old syntax (deprecated)
actionbutton btn 'Save {1}' parameters ['Hello'];
dynamictext txt (content 'Value: {1}' parameters ['test']);

-- New unified syntax
actionbutton btn 'Save {1}' with ({1} = 'Hello');
dynamictext txt (content 'Value: {1}' with ({1} = 'test'));
```

Benefits of `with` syntax:
1. **Explicit mapping** - Clear which placeholder gets which value
2. **Consistent** - Same syntax for microflows and pages
3. **Flexible ordering** - Can use `{1}` and `{3}` without `{2}`
4. **Self-documenting** - Placeholder purpose is clear from the mapping

## Best Practices

1. **Use meaningful data source names** - Name widgets descriptively (`dvOrder`, `lvItems`, `galleryProducts`)

2. **Be explicit about data sources** - Always qualify attributes with the widget name in nested contexts

3. **Keep templates readable** - For complex templates, format the `with` clause on multiple lines

4. **Prefer templates over concatenation** - `'Order {1}' with ({1} = $num)` is clearer than `'Order ' + $num`

5. **Use expressions for formatting** - Apply formatting in the expression: `with ({1} = formatDecimal($price, 2))`

## Related: Unified Parameter Syntax for CALL Statements

MDL uses a consistent parameter passing syntax across different call types. Parameter **names** do not use the `$` prefix (that's reserved for variable **values**).

### Microflow Calls

```sql
-- Parameter names without $ prefix
call microflow Module.ProcessOrder (OrderId = $Id, CustomerName = 'John');

-- With result variable
$Result = call microflow Module.Calculate (value = 100, Multiplier = 2);
```

### Java Action Calls

```sql
-- Same syntax as microflows
call java action CustomActivities.ExecuteOQL (OqlStatement = 'SELECT...');

-- With result
$count = call java action CustomActivities.CountRecords (EntityName = 'Order');
```

### Nanoflow Calls

```sql
call nanoflow Module.ValidateInput (Input = $FormData, Strict = true);
```

### Comparison with String Templates

| Context | Parameter Syntax | Example |
|---------|------------------|---------|
| String template | `{n} = expr` | `with ({1} = $Name)` |
| Microflow call | `Name = expr` | `(FirstName = 'John')` |
| Java action call | `Name = expr` | `(Statement = $query)` |

The key difference:
- **Templates** use positional `{1}`, `{2}` placeholders for text interpolation
- **Calls** use named parameters for function invocation
