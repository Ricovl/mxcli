# CREATE MESSAGE DEFINITION

## Synopsis

```text
CREATE [ OR MODIFY ] MESSAGE DEFINITION module.Name
    [ FOLDER 'folder_path' ]
    FOR module.Entity [ AS 'ExposedName' ] {
        AttributeName [ AS 'ExposedName' ] [ EXAMPLE 'text' ],
        module.Association/module.TargetEntity [ AS 'ExposedName' ] { ... },
        ...
    };
```

**Mendix 11.15 and later.** Below 11.15 the statement is refused; a message
definition is then an entry of a
[message definition collection](create-message-definition-collection.md).

## Description

Creates one message definition as its own document. Mendix 11.15 removed the
message definition collection and stores each definition as a separate
document. Studio Pro's converter turns a collection into a folder with the
collection's name, holding one document per definition, so `MD_Order` with the
definition `OrderMessage` becomes the folder `MD_Order` with the document
`OrderMessage` in it.

The body is the collection's `DEFINITION` entry: the same members, the same
rules for attributes and associations, the same derived cardinality. See
[CREATE MESSAGE DEFINITION COLLECTION](create-message-definition-collection.md#members).

A mapping refers to the document by its two-part name:

```sql
mdl 1;
create message definition Sales.OrderMessage
  folder 'Messages/MD_Order'
  for Sales.Order as 'Orders' {
    OrderId,
    Total as 'GrandTotal',
    Sales.OrderLine_Order/Sales.OrderLine as 'Lines' { Sku, Quantity },
    Sales.Order_Customer/Sales.Customer { FirstName, Address example 'Kerstraat 5' }
  };

create import mapping Sales.IMM_Order
  with message definition Sales.OrderMessage
{
  create Sales.Order { OrderId = OrderId }
};
```

On 11.15 a mapping stores this reference in `MessageDefinition2`. A script
written for an older version can still name a converted definition in three
parts (`Sales.MD_Order.OrderMessage`): it resolves to the document in the folder
`MD_Order`, and the mapping stores the two-part name.

## Editing one without restating it

```sql
mdl 1;
alter message definition Sales.OrderMessage add member LastName in Customer;
alter message definition Sales.OrderMessage set member Total as 'GrandTotal';
alter message definition Sales.OrderMessage drop member Sku in Lines;
```

These are the collection's member edits, addressed by the document's two-part
name.

## Other statements

```text
LIST MESSAGE DEFINITIONS [ IN module ];
DESCRIBE MESSAGE DEFINITION module.Name;
DROP MESSAGE DEFINITION [ IF EXISTS ] module.Name;
```

`DESCRIBE` writes this statement, so describe and exec round-trip. Dropping a
definition that a mapping still uses is refused, and the mappings are named.

On 11.15 the collection statements (`CREATE`, `ALTER`, `DESCRIBE` and `DROP
MESSAGE DEFINITION COLLECTION`) are refused with a hint to use this form, and
`LIST MESSAGE DEFINITION COLLECTIONS` points to `LIST MESSAGE DEFINITIONS`.

## See Also

[CREATE MESSAGE DEFINITION COLLECTION](create-message-definition-collection.md), [CREATE IMPORT MAPPING](create-import-mapping.md), [CREATE EXPORT MAPPING](create-export-mapping.md)
