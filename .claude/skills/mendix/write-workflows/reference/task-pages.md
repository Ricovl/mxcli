# Workflow task pages — reference

A task page is how a user finishes a task, so it needs a button for every
outcome. Mendix does not add them: a page that only shows the task leaves every
instance waiting at it, and mx check and the build pass. `mxcli check` warns
**MDL-WF20** when a task's page has no button that completes one of its outcomes
(a `complete task` button, or a microflow that reaches `set task outcome`). The button action is
the page's own **`complete task '<Outcome>'`**; no microflow is needed to complete
a task.

## The user must claim the task first

Targeting decides who may see a task; it does not assign it. Measured on the
11.13.0 runtime (Prototype security, the targeted user in a browser): a `complete
task` button on a task the user has not claimed shows "You can't complete this
user task, it is not assigned to you." and the task stays open. With an *Assign to
me* button clicked first, the same button completes it, the outcome's branch runs
and the workflow moves on. So a task page has one shared claim microflow and one
button per outcome.

## Template

```sql
mdl 1;
-- One per app, shared by every task page.
create microflow Approvals.ACT_Task_AssignToMe ($WorkflowUserTask: System.WorkflowUserTask)
begin
  change $WorkflowUserTask (System.WorkflowUserTask_Assignees = [%CurrentUser%]);
  commit $WorkflowUserTask;
end;

create page Approvals.WF_ReviewRequest
(
  params: ( $WorkflowUserTask: System.WorkflowUserTask ),
  title: 'Review request',
  layout: Atlas_Core.Atlas_Default
)
{
  dataview dvTask (DataSource: $WorkflowUserTask) {
    dynamictext txtTask (Content: '{1}', ContentParams: ({1} = Name))
    actionbutton btnAssign (Caption: 'Assign to me',
      Action: call microflow Approvals.ACT_Task_AssignToMe(WorkflowUserTask = $WorkflowUserTask))
    actionbutton btnApprove (Caption: 'Approve', Action: complete task 'Approve', ButtonStyle: Success)
    actionbutton btnReject (Caption: 'Reject', Action: complete task 'Reject', ButtonStyle: Danger)
  }
};

grant view on page Approvals.WF_ReviewRequest to Approvals.User;
grant execute on microflow Approvals.ACT_Task_AssignToMe to Approvals.User;
```

It builds clean on 11.15.0 (ako/mxcli#1709).

- **The outcome names must match the user task's** (`outcomes 'Approve' { … }
  'Reject' { … }`), one button each.
- A task whose targeting can only yield one user can use `auto assign single
  user` on the user task instead of the claim button.
- To show the context object, give the page a second parameter of the context
  entity alongside `$WorkflowUserTask`.

## Opening a task, a workflow, or starting one from a page

The other workflow events are button actions too, in the words their microflow
actions use:

```sql
-- A task inbox: the row, or a link button in a column, opens the task's page.
datagrid dgTasks (DataSource: database from System.WorkflowUserTask
    where [System.WorkflowUserTask_Assignees = '[%CurrentUser%]'], onClick: open user task) {
  column (Attribute: Name, Caption: 'Task')
  column (Attribute: Name, Caption: ' ', ShowContentAs: customContent, Sortable: false) {
    linkbutton btnOpen (Caption: 'Open', Action: open user task)
  }
}
-- Admin: the workflow's own admin page if it overrides one, else DefaultPage
-- (required: CE1834 without it). The page takes a System.Workflow parameter.
linkbutton btnAdmin (Caption: 'Details', Action: open workflow with (DefaultPage: Approvals.WF_Admin))
-- Start a workflow on the data container's object.
actionbutton btnSubmit (Caption: 'Submit', Action: call workflow Approvals.WF_Request with (ClosePage: false))
```

- `open user task` claims the task as it opens it (Studio Pro's "Auto-assign");
  `with (AssignOnOpen: false)` does not. `OpenWhenAssigned: true` lets any user
  with access open a task someone else has claimed.
- `open user task` belongs in a `System.WorkflowUserTask` data container and
  `open workflow` in a `System.Workflow` one; anywhere else mx check reports CE1843.
- `complete task` and `call workflow` close the page unless `ClosePage: false`;
  `complete task` also commits unless `Commit: false`.

## Keep the microflows to what the workflow has to do

A workflow cannot change an object itself, so an outcome that must write data, or
a timer that must notify someone, does need a `call microflow`. But do not add one
per outcome only to mirror progress into a status attribute: `System.Workflow` and
`System.WorkflowUserTask` already record the state and the outcome chosen, and the
task page's `complete task` buttons need none. Add a status attribute and its
microflows only when the prompt asks for one.
