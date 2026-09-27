# testapp-views: a Studio Pro-authored view entity

A trimmed copy of `ako/TestApp`, a Mendix **11.14.0** app (MPR v2) authored in
Studio Pro. It exists for one thing PedApp (`testdata/pedapp`) does not have: a
**view entity mxcli did not write**, `MyFirstModule.VCar`, over the persistent
entity `MyFirstModule.Car`.

On an element mxcli created, `GUID == $ID` from birth, so a test that re-mints
the GUID from the `$ID` reproduces the same value and cannot fail (CLAUDE.md, "A
`GUID` Is the Database's Identity"). VCar and its attributes have `GUID != $ID`.
It is used by `mdl/roundtrip/viewentity_identity_test.go` (#731).

## What was kept, and what was left out

- **Kept verbatim:** every unit file that belongs to the project root or to
  `MyFirstModule` (its domain model, security, pages, microflows, the view-entity
  source document, and the project-level security, navigation and settings).
  Do not edit them.
- **Left out:** the units of every other module (Administration, Atlas_*,
  WorkflowCommons, …) with their rows in `TestApp.mpr`'s `Unit` table, the
  17 MB `Projects$ModuleGuidMapping` unit, and `javasource/`,
  `javascriptsource/`, `theme/`, `themesource/`, `widgets/`. mxcli opens and
  describes the result; it is not meant to build with mxbuild.

## Provenance

Copied from the `ako/TestApp` clone on 2026-09-27; the trim removed rows from the
`Unit` table and the matching files under `mprcontents/`, and nothing else.
