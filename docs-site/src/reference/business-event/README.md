# Business Event Statements

Statements for managing business event services.

Business event services enable asynchronous, event-driven communication between Mendix applications (and external systems) using the Mendix Business Events platform. A service defines named messages with typed attributes and specifies whether the application publishes or subscribes to each message.

## Statements

| Statement | Description |
|-----------|-------------|
| [CREATE BUSINESS EVENT SERVICE](create-business-event-service.md) | Define a business event service with messages |
| [DROP BUSINESS EVENT SERVICE](drop-business-event-service.md) | Remove a business event service |

## Related Statements

| Statement | Syntax |
|-----------|--------|
| Show business events | `LIST BUSINESS EVENTS [IN module]` |
| Describe service | `DESCRIBE BUSINESS EVENT SERVICE module.Name` |
