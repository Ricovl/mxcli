# Deprecated-alias examples

The scripts in this directory **deliberately use deprecated MDL spellings**
(the `MDL-DEPRnnn` entries of `mdl/deprecation`). Every other script under
`mdl-examples/` is written in the canonical form and is held to it by the
conformance gate (`make check-conformance`, ako/mxcli#756).

They exist because `mxcli fmt --upgrade` needs a corpus of old spellings to be
proven on:

- `mdl/upgrade`'s `TestUpgrade_ExamplesKeepTheirStatements` upgrades every
  example and requires the result to build the same statements;
- `mdl/roundtrip`'s `TestUpgradeExecutesToTheSameModel` (integration) executes
  the original and the upgrade on two copies of PedApp and compares the models.

Each file is a verbatim copy, taken when mdl-examples was migrated to the
canonical form, of the script named by its file name (`<dir>--<name>.mdl` is
`mdl-examples/<dir>/<name>.mdl` before `fmt --upgrade`). Together they cover
every registered spelling the corpus used at that point.

Rules:

- Do not run `fmt --upgrade` here, and do not copy these spellings elsewhere.
- Every script here must parse and use at least one deprecated spelling
  (`TestAliasExamplesExerciseAliases` in `mdl/conformance`). One that no longer
  does has lost its reason to be here.
- A new deprecated spelling's execute-both coverage goes here, as a script that
  uses it.
