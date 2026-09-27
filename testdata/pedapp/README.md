# PedApp: the Studio Pro-authored round-trip fixture

PedApp is a Mendix **11.13.0** app (MPR v2), authored in Studio Pro from the
standard starter app and marketplace modules. It is the fixture for the
round-trip harness in `mdl/roundtrip/` (ADR-0012, plan items 0.1 and 0.2 of
`docs/11-proposals/PROPOSAL_mdl_beta_syntax_freeze.md`).

Its value is that **mxcli did not write it**. In content mxcli wrote, an element's
`GUID` equals its `$ID`, so identity loss cannot be detected (see CLAUDE.md, "A
`GUID` Is the Database's Identity"). Every other round-trip test in the repo
runs on mxcli-authored content.

## Contents

| Module | Source |
|---|---|
| Administration | Marketplace v4.3.2 |
| Atlas_Core | Marketplace v4.1.3 |
| Atlas_Web_Content | Marketplace v4.1.0 |
| DataWidgets | Marketplace v3.5.0 |
| FeedbackModule | Marketplace v4.0.2 |
| MyFirstModule | Studio Pro starter |
| NanoflowCommons | Marketplace v6.0.0 |
| WebActions | Marketplace v2.11.0 |

It has microflows, nanoflows, pages, snippets, layouts, building blocks, menus,
Java and JavaScript actions, JSON structures, mappings, image collections,
module and user roles, and translated texts (en_US, nl_NL and others).

## What was kept, and what was left out

- **Kept verbatim:** `PedApp.mpr` and `mprcontents/`. Do not edit them.
- **`widgets/`:** each `.mpk` is stripped to its XML files (`package.xml` and the
  widget definitions). That is all mxcli reads to build a pluggable widget; the
  JavaScript bundles and images would add about 10 MB.
- **Left out:** `deployment/`, `theme-cache/`, `.mendix-cache/`, `javasource/`,
  `javascriptsource/`, `themesource/`, `theme/`, `resources/` and `userlib/`.
  None of them is model content.

## Provenance

Many copies of PedApp exist on the development machine, and most were changed
by mxcli experiments (modules such as `AN`, `SR`, `FU`, or edited
documents). This copy was chosen because its `mprcontents/` and `.mpr` are
byte-identical across five independent copies. It has only the eight modules
above, and no mxcli-authored module.

## Rules

- **Never connect mxcli to this directory to write.** Tests copy it first
  (`mdl/roundtrip` works on a temporary copy). A write here corrupts the
  baseline of every round-trip measurement.
- **Add documents only in Studio Pro**, never with mxcli. Plan item 0.2 asks for a
  workflow and REST documents; they have to be authored in Studio Pro and
  copied in, and the allowlist re-measured.
- Adding a document changes what the harness enumerates, so re-run
  `go test -tags integration -v -run TestPedAppRoundTrip ./mdl/roundtrip/` and
  update `mdl/roundtrip/allowlist_test.go` in the same change.
