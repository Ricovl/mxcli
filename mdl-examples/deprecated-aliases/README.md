# Deprecated-alias examples

The scripts in this directory **deliberately use deprecated MDL spellings**
(the `MDL-DEPRnnn` entries of `mdl/deprecation`) or **constructs whose meaning
the `mdl 1;` header changes** (the `MDL-V1-*` rules of `mdl/langver`), and they
have no language header: they are mdl 0. Every other script under
`mdl-examples/` is written in the canonical form, starts with `mdl 1;`, and is
held to both by the conformance gate (`make check-conformance`, ako/mxcli#756
and decision 1 on ako/mxcli#714).

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

When mdl-examples moved to `mdl 1;` (`fmt --upgrade --header`), copies were
added for the header-gated rewrites the corpus used that no copy here covered:
`MDL-V1-SET` (`f1-13-bare-variable-assignment`, `additive-operator-order`),
`MDL-V1-ESCAPE` (`264-log-node-expression-roundtrip`,
`captrack-expression-literal-tab`) and the string forms of `MDL-V1-LIST`
(`ledger-53-string-contains`, `ledger-63-string-find`). The execute-both test
proves the upgrade keeps their meaning; `MDL-V1-QUOTEDEXPR` is covered by
`bug-tests/836-quoted-expression-mdl0.mdl`, which stays headerless in place.

Rules:

- Do not run `fmt --upgrade` here, and do not copy these spellings elsewhere.
- Every script here must parse and use at least one deprecated spelling or one
  header-gated construct other than a statement terminator
  (`TestAliasExamplesExerciseAliases` in `mdl/conformance`). One that no longer
  does has lost its reason to be here.
- A new deprecated spelling's execute-both coverage goes here, as a script that
  uses it.
