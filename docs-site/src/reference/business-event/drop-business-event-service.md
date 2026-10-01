# DROP BUSINESS EVENT SERVICE

## Synopsis

```sql
DROP BUSINESS EVENT SERVICE module.Name;
```

## Description

Removes a business event service from the project. The service is identified by its qualified name. If the service does not exist, an error is returned.

Dropping a service removes all its message definitions and operation implementations. Any microflows that reference the dropped service's messages will need to be updated.

## Parameters

`module.Name`
:   The qualified name of the business event service to drop (`Module.ServiceName`).

## Examples

```sql
DROP BUSINESS EVENT SERVICE Shop.OrderEvents;
```

## See Also

[CREATE BUSINESS EVENT SERVICE](create-business-event-service.md)
