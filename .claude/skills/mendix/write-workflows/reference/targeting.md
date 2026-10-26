# Targeting a user task — reference

A `targeting users xpath` constraint is evaluated **on the user** — `System.User`,
or the specialization App Settings > Workflows names — so it names the user's own
members, with no entity in front. `targeting groups xpath` is evaluated on
`System.WorkflowGroup`. To send a task to everyone holding a user role:

```sql
user task Review 'Review' page Module.WF_Review
  targeting users xpath [System.UserRoles = '[%UserRole_Manager%]']
  outcomes 'Approve' { } 'Reject' { };
```

## Measured

With mx check on 11.15.0, and for the role form also at runtime on 11.13.0:

| Constraint | Result |
|---|---|
| `[System.UserRoles = '[%UserRole_User%]']` | clean; selects the role's users |
| `[System.UserRoles/System.UserRole/Name = 'User']` | clean |
| `[Name = 'MxAdmin']`, `[Active = true()]` | clean |
| `[System.User/Name = 'MxAdmin']` — the entity as a path step | **CE0161** |
| `[NoSuchAttr = 'x']` | **CE0161** |
| `[System.UserRoles = '[%UserRole_Manager%]']` with no `Manager` role | **CE1613** |
| groups: `[Name = 'Reviewers']` / `[System.WorkflowGroup/Name = …]` | clean / **CE0161** |

`check -p --references` and `exec` report the two CE0161 shapes, and the same
leading entity step in a microflow retrieve or a list view's database source
(ako/mxcli#1711). The role token must name a user role the project has (`show user
roles`), spelled with the same case: `check -p --references` and `exec` refuse
one that does not, in any XPath constraint (ako/mxcli#1723). It matters beyond the
one error — while any CE1613 is in the app, mx check reports **no CE0161 at all**.
A constraint on one hard-coded user is a test shortcut, not targeting.
