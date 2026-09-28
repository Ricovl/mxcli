# CREATE WORKFLOW

## Synopsis

```sql
CREATE [ OR MODIFY ] WORKFLOW module.Name
    PARAMETER $Variable: module.Entity
    [ OVERVIEW PAGE module.PageName ]
    [ DISPLAY 'caption' ]
    [ DESCRIPTION 'text' ]
BEGIN
    activities
END WORKFLOW
```

## Description

Creates a workflow that models a long-running business process. Each workflow has a context parameter (an entity that carries the data through the process) and a sequence of activities.

The `OR MODIFY` option updates an existing workflow if one exists with the same name, or creates a new one if it does not.

### Activity Types

The following activity types can appear inside a workflow body:

**USER TASK**
:   An activity that requires human action. A user task displays a page to an assigned user and waits for them to select an outcome. Supports targeting by microflow or XPath to determine the assignee.

```sql
USER TASK name 'caption'
    [ PAGE module.PageName ]
    [ TARGETING MICROFLOW module.MicroflowName ]
    [ TARGETING [ USERS | GROUPS ] XPATH [ xpath ] ]
    [ ENTITY module.Entity ]
    [ DUE DATE 'expression' ]
    [ DESCRIPTION 'text' ]
    OUTCOMES 'OutcomeName' { activities } [ 'OutcomeName' { activities } ... ]
```

The targeting XPath is written in literal brackets, like every XPath in MDL, so quotes inside it are written once: `TARGETING USERS XPATH [System.UserRoles = '[%UserRole_Manager%]']`. The quoted form `XPATH '[...]'` still parses and warns `MDL-DEPR031`; `mxcli fmt --upgrade` rewrites it.

**MULTI USER TASK**
:   Same as USER TASK but assigned to multiple users. Uses the same syntax with `MULTI USER TASK` instead of `USER TASK`.

**CALL MICROFLOW**
:   Calls a microflow as part of the workflow. The microflow can return a Boolean or enumeration to drive conditional outcomes.

```sql
CALL MICROFLOW module.Name [ 'caption' ]
    [ WITH ( Parameter = expression [, ...] ) ]
    [ OUTCOMES 'OutcomeName' { activities } ... ]
```

**CALL WORKFLOW**
:   Calls a sub-workflow.

```sql
CALL WORKFLOW module.Name [ 'caption' ]
    [ WITH ( Parameter = expression [, ...] ) ]
```

**DECISION**
:   A conditional branch based on an expression. Each outcome maps to a different execution path.

```sql
DECISION [ 'caption' ]
    OUTCOMES 'OutcomeName' { activities } [ 'OutcomeName' { activities } ... ]
```

**PARALLEL SPLIT**
:   Splits the workflow into parallel paths that execute concurrently and rejoin before continuing.

```sql
PARALLEL SPLIT [ 'caption' ]
    PATH 1 { activities }
    PATH 2 { activities }
    [ PATH n { activities } ... ]
```

**JUMP TO**
:   Unconditionally jumps to a named activity earlier in the workflow, creating a loop.

```sql
JUMP TO activity_name [ 'caption' ]
```

**WAIT FOR TIMER**
:   Pauses the workflow until a timer expression elapses.

```sql
WAIT FOR TIMER [ 'duration_expression' ]
```

**WAIT FOR NOTIFICATION**
:   Pauses the workflow until an external notification resumes it.

```sql
WAIT FOR NOTIFICATION
```

**END**
:   Explicitly terminates a workflow path.

```sql
END
```

### Boundary Events

User tasks, call microflow activities, and wait-for-notification activities support boundary events that trigger alternative flows when a timer expires:

```sql
USER TASK name 'caption'
    PAGE module.Page
    OUTCOMES 'Done' { }
    BOUNDARY EVENT InterruptingTimer '${PT1H}' { activities }
    BOUNDARY EVENT NonInterruptingTimer '${PT30M}' { activities }
```

## Parameters

`module.Name`
:   Qualified name of the workflow (`Module.WorkflowName`).

`PARAMETER $Variable: module.Entity`
:   The context parameter. The variable name (prefixed with `$`) and entity type define the data that flows through the workflow. The entity must already exist.

`OVERVIEW PAGE module.PageName`
:   Optional overview page displayed when viewing workflow instances.

`DISPLAY 'caption'`
:   Optional display name shown in the workflow overview.

`DESCRIPTION 'text'`
:   Optional description of the workflow's purpose.

## Examples

Simple approval workflow:

```sql
CREATE WORKFLOW HR.ApprovalFlow
    PARAMETER $Context: HR.LeaveRequest
    OVERVIEW PAGE HR.WorkflowOverview
BEGIN
    USER TASK ReviewTask 'Review the request'
        PAGE HR.ReviewPage
        OUTCOMES 'Approve' { } 'Reject' { };
END WORKFLOW;
```

Workflow with decision and parallel paths:

```sql
CREATE WORKFLOW Sales.OrderProcessing
    PARAMETER $Context: Sales.Order
BEGIN
    CALL MICROFLOW Sales.ACT_ValidateOrder 'Validate'
        OUTCOMES 'True' {
            PARALLEL SPLIT
                PATH 1 {
                    CALL MICROFLOW Sales.ACT_ReserveInventory;
                }
                PATH 2 {
                    CALL MICROFLOW Sales.ACT_ProcessPayment;
                };

            USER TASK ShipTask 'Arrange shipping'
                PAGE Sales.ShippingPage
                TARGETING MICROFLOW Sales.ACT_GetWarehouseStaff
                OUTCOMES 'Shipped' { };
        }
        'False' {
            CALL MICROFLOW Sales.ACT_NotifyCustomer;
        };
END WORKFLOW;
```

Workflow with a timer boundary event:

```sql
CREATE WORKFLOW Support.TicketEscalation
    PARAMETER $Context: Support.Ticket
BEGIN
    USER TASK AssignTask 'Handle ticket'
        PAGE Support.TicketPage
        OUTCOMES 'Resolved' { }
        BOUNDARY EVENT InterruptingTimer '${PT24H}' {
            CALL MICROFLOW Support.ACT_EscalateTicket;
        };
END WORKFLOW;
```

## See Also

[DROP WORKFLOW](drop-workflow.md)
