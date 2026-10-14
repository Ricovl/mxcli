# What `check -p --references` resolves

Beyond the page, microflow, entity and icon names a statement names directly,
`check -p --references` resolves these against the project (mendixlabs/mxcli#1472):

- an `ALTER PAGE` `INSERT` / `REPLACE` / `DROP` target, and the flows, pages and
  snippets the inserted or replacing widgets name (a `SET` target is dry-run
  against the stored page);
- a DataGrid 2 column's `Attribute:` on a `database` grid;
- an access rule's `where […]` members;
- an entity event handler's microflow;
- `Module.Enum.Key` in an expression;
- a view entity's FROM / JOIN entities.

## Not yet resolved

A passing check does not prove these; `mxcli docker check` is still the gate:

- a built-in input widget's `Attribute:` (`textbox (Attribute: Typo)`);
- any attribute inside an `ALTER PAGE` insert;
- an enumeration key inside a page widget's expression.
