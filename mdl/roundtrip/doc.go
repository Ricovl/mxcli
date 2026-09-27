// SPDX-License-Identifier: Apache-2.0

// Package roundtrip holds the round-trip harness over the committed Studio
// Pro-authored fixture (testdata/pedapp). It has no non-test code: the harness
// is an integration test (`-tags integration`) that enforces the two
// round-trip laws of ADR-0012 on every document the fixture contains.
//
//   - GetPut: executing a document's own `describe` output, unchanged, writes
//     nothing.
//   - PutGet: describing the document again returns what was described before.
//
// It also holds the execute-both property test of `mxcli fmt --upgrade`
// (upgrade_property_test.go): every mdl-examples script and its upgrade run on
// two copies of the fixture and must write the same model.
//
// Every other round-trip test in the repo runs on mxcli-authored content, where
// an element's GUID equals its $ID, so identity loss cannot show up there. This
// fixture was authored by Studio Pro and the marketplace, so it can.
//
// See docs/13-decisions/0012-mdl-first-and-data-first-editing.md and
// docs/11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md §8.1, §8.4, §9 (0.1, 0.2).
package roundtrip
